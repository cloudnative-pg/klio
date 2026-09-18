/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package consumer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// maintainTier1 applies the tier1 retention policy and drops the WAL files
// that are no longer required by any remaining tier1 backup. It records the
// tier1 maintenance metric; the error is returned for logging but is
// best-effort (the caller does not fail the task on it).
func (d *Backup) maintainTier1(ctx context.Context, task *queue.BackupTask) error {
	log.FromContext(ctx).Info("Applying tier1 maintenance", "cluster", task.ClusterName)

	err := d.runTier1Retention(ctx, task)
	recordMaintenance(ctx, task.ClusterName, opentelemetry.Tier1, err)

	return err
}

func (d *Backup) runTier1Retention(ctx context.Context, task *queue.BackupTask) error {
	clusterName := task.ClusterName

	// The cluster name reaches us from the client's CloseBackup request via the
	// queue task and is used below as a WAL directory path, so validate it
	// before we touch the filesystem. This guard used to live in the gRPC
	// SetFirstRequiredWAL handler that drove retention before it moved here.
	if err := repository.ValidatePathComponent(clusterName); err != nil {
		return fmt.Errorf("invalid cluster name %q: %w", clusterName, err)
	}

	// List tier1 backups and snapshots once and share them between the steps
	// below: listing separately let two of them see different,
	// concurrently-changing snapshots of the same cluster, the same race class
	// fixed for tier1-vs-tier2 in aa7f5879.
	tier1Backups, err := d.tier1Client.ListBackups(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("while listing tier1 backups for cluster %q: %w", clusterName, err)
	}

	tier1Snapshots, err := d.listManifests(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("while listing tier1 snapshots for cluster %q: %w", clusterName, err)
	}

	// Delete tier1 snapshot parts that never got a metadata snapshot (the
	// backup died between uploading its parts and closing), once a newer
	// backup proves they were abandoned. Best-effort: a failure here must not
	// block the retention and WAL cleanup below for this cycle.
	if err := deleteAbandonedOrphans(ctx, d.tier1Client, clusterName, tier1Snapshots, tier1Backups); err != nil {
		log.FromContext(ctx).Error(err, "Error while deleting abandoned orphan backups, skipping")
	}

	// Delete the tier1 base backups that fall outside the retention policy,
	// while never deleting one that has not yet reached tier2, so no base backup
	// is lost before it is durable on tier2.
	keep, err := d.tier1RetentionGuard(ctx, clusterName, tier1Snapshots, tier1Backups)
	if err != nil {
		return err
	}

	if err := d.applyRetention(
		ctx, d.tier1Client, clusterName, tier1Backups, task.Tier1RetentionPolicy, keep,
	); err != nil {
		return fmt.Errorf("while applying tier1 retention policy: %w", err)
	}

	return d.applyTier1WALRetention(ctx, clusterName)
}

// deleteAbandonedOrphans deletes snapshot parts of a backup that never
// received a metadata snapshot on this tier, once a newer backup has reached
// this tier successfully. A backup's metadata snapshot is the last part
// written on each tier (tier1: klioclient.BackupExecutor.Close uploads it
// last; tier2: relayTier2's MigrateSnapshots call can likewise migrate some
// parts and not others before failing), so a backup that dies partway
// through leaves parts with no metadata behind on that tier: invisible to
// ListBackups there and therefore never retention-managed on it.
//
// Klio processes one backup at a time per cluster, so a newer backup
// reaching a real, catalogued completion on this tier proves the
// metadata-less one is not still arriving: it is a permanently abandoned
// attempt, the same newer-backup-proves-it trick keepUntilOnTier2 uses for
// the tier1-vs-tier2 relay guard. If no catalogued backup exists on this
// tier yet, nothing is deletable, since the orphan could still be genuinely
// in progress (tier1: still uploading; tier2: still relaying).
func deleteAbandonedOrphans(
	ctx context.Context,
	client retentionClient,
	clusterName string,
	snapshots []kopia.Manifest,
	backups klioclient.BackupList,
) error {
	contextLogger := log.FromContext(ctx)

	known, newestKnownGood := knownGoodBackups(backups)
	if newestKnownGood == 0 {
		contextLogger.Info("No catalogued backup on this tier yet, skipping orphan cleanup",
			"cluster", clusterName)

		return nil
	}

	var candidates int
	var errs error

	for i := range snapshots {
		name := snapshots[i].Tags[klioclient.BackupNameTagName]
		if name == "" || known.Has(name) {
			continue
		}

		candidates++

		startedAt := snapshots[i].StartTime.ToTime().Unix()
		if startedAt >= newestKnownGood {
			contextLogger.Info("Orphan snapshot may still be in progress, keeping it",
				"cluster", clusterName, "backup", name, "snapshotID", snapshots[i].ID, "startedAt", startedAt)

			continue
		}

		contextLogger.Info("Deleting orphan snapshot with no metadata snapshot",
			"cluster", clusterName, "backup", name, "snapshotID", snapshots[i].ID, "startedAt", startedAt)
		if err := client.DeleteSnapshot(ctx, snapshots[i].ID); err != nil {
			errs = errors.Join(errs, fmt.Errorf(
				"while deleting orphan snapshot %q of backup %q: %w", snapshots[i].ID, name, err))
		}
	}

	contextLogger.Info("Checked for abandoned orphan snapshots",
		"cluster", clusterName, "orphanCandidates", candidates, "newestKnownGoodStartedAt", newestKnownGood)

	return errs
}

// knownGoodBackups returns the set of backup names that have a metadata
// snapshot on this tier, and the most recent StartedAt among them (0 if
// none).
func knownGoodBackups(backups klioclient.BackupList) (*stringset.Data, int64) {
	known := stringset.New()

	var newest int64
	for i := range backups {
		known.Put(backups[i].Name)
		newest = max(newest, backups[i].StartedAt)
	}

	return known, newest
}

// groupSnapshotsByBackup partitions raw snapshot manifests by the backup name
// recorded in their tags, discarding any manifest with no backup name (not
// one of ours, or a metadata-less fragment with no way to attribute it).
func groupSnapshotsByBackup(snapshots []kopia.Manifest) map[string][]kopia.Manifest {
	groups := make(map[string][]kopia.Manifest)
	for i := range snapshots {
		name := snapshots[i].Tags[klioclient.BackupNameTagName]
		if name == "" {
			continue
		}

		groups[name] = append(groups[name], snapshots[i])
	}

	return groups
}

// tier1RetentionGuard returns a predicate that reports whether a tier1 backup
// must be kept because it has not yet been fully migrated to tier2. When tier2
// is not configured there is nothing to protect and the predicate is nil.
// tier1Snapshots and tier1Backups are the caller's already-listed tier1
// state, so the guard's view of "what exists on tier1" agrees with the one
// applyRetention evaluates the policy against.
func (d *Backup) tier1RetentionGuard(
	ctx context.Context,
	clusterName string,
	tier1Snapshots []kopia.Manifest,
	tier1Backups klioclient.BackupList,
) (func(backup *klioclient.BackupMetadata) bool, error) {
	contextLogger := log.FromContext(ctx)

	if !d.tier2Enabled {
		contextLogger.Info("Tier2 disabled on this server, tier1 retention runs unguarded",
			"cluster", clusterName)

		return nil, nil
	}

	tier2Snapshots, err := d.tier2Kopia.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf(
			"while listing tier2 snapshots to guard tier1 retention for cluster %q: %w", clusterName, err)
	}

	contextLogger.Info("Guarding tier1 retention against the tier2 relay state",
		"cluster", clusterName, "tier1Snapshots", len(tier1Snapshots), "tier2Snapshots", len(tier2Snapshots))

	return keepUntilOnTier2(tier1Snapshots, tier2Snapshots, tier1Backups), nil
}

// keepUntilOnTier2 returns a predicate that reports whether a tier1 backup must
// be kept because it has not been relayed to tier2 yet. The relay is a single
// snapshot migration with no ordering between a backup's parts, so the tier2
// metadata snapshot alone does not prove the data is there: a backup is
// deletable only when every tier1 snapshot of it has a counterpart on tier2.
//
// A backup with no snapshot at all on tier2 is either not relayed yet or was
// relayed and then deleted by tier2 retention. The relay migrates every tier1
// snapshot of the cluster at once, so a newer backup complete on tier2 proves
// the relay ran after the older one existed: such a backup is deletable, or
// tier1 would keep it (and its WALs) forever. A backup the client never meant
// to relay has nothing to wait for.
func keepUntilOnTier2(
	tier1Snapshots, tier2Snapshots []kopia.Manifest,
	tier1Backups klioclient.BackupList,
) func(backup *klioclient.BackupMetadata) bool {
	state := newRelayState(tier1Snapshots, tier2Snapshots)

	var newestComplete int64
	for i := range tier1Backups {
		if state.complete(tier1Backups[i].Name) {
			newestComplete = max(newestComplete, tier1Backups[i].StartedAt)
		}
	}

	return func(backup *klioclient.BackupMetadata) bool {
		if backup.Annotations[klioclient.Tier2RelayAnnotationName] == klioclient.Tier2RelaySkipped {
			return false
		}

		// No tier1 snapshot means nothing to protect.
		if state.parts[backup.Name] == 0 || state.complete(backup.Name) {
			return false
		}

		// Partially on tier2: a relay is in flight or failed midway.
		if state.relayed[backup.Name] > 0 {
			return true
		}

		// Absent from tier2: relayed and deleted there only if a newer backup
		// went through the relay.
		return newestComplete == 0 || backup.StartedAt >= newestComplete
	}
}

// relayState counts, per backup name, the tier1 snapshots and how many of
// them have a counterpart on tier2.
type relayState struct {
	parts   map[string]int
	relayed map[string]int
}

func newRelayState(tier1Snapshots, tier2Snapshots []kopia.Manifest) relayState {
	onTier2 := stringset.New()
	for i := range tier2Snapshots {
		onTier2.Put(snapshotPartKey(tier2Snapshots[i]))
	}

	state := relayState{parts: make(map[string]int), relayed: make(map[string]int)}
	for name, parts := range groupSnapshotsByBackup(tier1Snapshots) {
		state.parts[name] = len(parts)
		for _, part := range parts {
			if onTier2.Has(snapshotPartKey(part)) {
				state.relayed[name]++
			}
		}
	}

	return state
}

// complete reports whether every tier1 snapshot of the backup is on tier2.
func (s relayState) complete(name string) bool {
	return s.parts[name] > 0 && s.relayed[name] == s.parts[name]
}

// snapshotPartKey identifies one part of a backup (pgdata, metadata, control
// data or a tablespace) independently of the repository it lives in. The
// migration preserves both the source and the tags, so the key matches across
// tiers.
func snapshotPartKey(m kopia.Manifest) string {
	return strings.Join([]string{
		m.Source.Host,
		m.Source.Path,
		m.Tags[klioclient.BackupNameTagName],
		m.Tags[klioclient.BackupContentTagName],
		m.Tags[klioclient.TablespaceNameTagName],
	}, "\x00")
}

// applyTier1WALRetention drops the tier1 WAL files that are no longer required
// by any remaining tier1 backup, clamped to the tier2 transfer frontier so
// WALs still pending upload are never deleted.
func (d *Backup) applyTier1WALRetention(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)

	if d.opts.Tier1WALRepository == nil {
		contextLogger.Info("Tier1 WAL repository not configured; skipping tier1 WAL retention")
		return nil
	}

	// Recompute the oldest in-use WAL from the backups that survived the
	// retention policy.
	backups, err := d.tier1Client.ListBackups(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("while listing tier1 backups for cluster %q: %w", clusterName, err)
	}

	if len(backups) == 0 {
		contextLogger.Info("No tier1 backups found; skipping tier1 WAL retention")
		return nil
	}

	oldestWAL := findOldestWAL(backups)
	if oldestWAL == "" {
		contextLogger.Info("Backups exist but none contain a StartWAL; skipping tier1 WAL retention")
		return nil
	}

	firstRequiredWAL, err := d.clampToTier2Frontier(ctx, clusterName, oldestWAL)
	if err != nil {
		return err
	}

	if firstRequiredWAL == "" {
		contextLogger.Info(
			"No tier2 transfer frontier recorded; skipping tier1 WAL retention to avoid deleting pending WALs",
			"clusterName", clusterName,
		)

		return nil
	}

	// SetFirstRequiredOnCluster slices the WAL name without a length check, so
	// validate it before handing it over (another guard inherited from the
	// removed gRPC SetFirstRequiredWAL handler).
	if err := repository.ValidateWalFileName(firstRequiredWAL); err != nil {
		return fmt.Errorf("computed first required WAL %q is invalid: %w", firstRequiredWAL, err)
	}

	contextLogger.Info("Applying tier1 WAL retention", "clusterName", clusterName, "firstRequiredWAL", firstRequiredWAL)
	if err := d.opts.Tier1WALRepository.SetFirstRequiredOnCluster(ctx, clusterName, firstRequiredWAL); err != nil {
		return fmt.Errorf("while applying tier1 WAL retention: %w", err)
	}

	return nil
}

// clampToTier2Frontier clamps the requested first-required WAL so that WAL
// files which have not yet been transferred to tier2 are never deleted from
// tier1. This is the single owner of that safety invariant now that retention
// runs entirely server-side.
//
// When tier2 is configured it returns an empty string if no transfer frontier
// has been recorded yet, in which case the caller must not delete anything.
//
// NOTE: for tier1-only deployments there is no tier2 to protect, so the
// requested WAL is applied directly and WAL retention is enforced.
func (d *Backup) clampToTier2Frontier(ctx context.Context, clusterName, firstRequiredWAL string) (string, error) {
	// Without tier2 there is no transfer frontier to protect, so the
	// requested WAL can be applied directly.
	if !d.tier2Enabled || d.opts.Queue == nil {
		return firstRequiredWAL, nil
	}

	latestUploadedWAL, err := d.opts.Queue.GetLatestUploadedWAL(ctx, clusterName)
	if err != nil {
		return "", fmt.Errorf("while checking latest uploaded WAL for cluster %q: %w", clusterName, err)
	}

	clamped := clampWAL(firstRequiredWAL, latestUploadedWAL)
	log.FromContext(ctx).Info("Clamping tier1 WAL retention to tier2 frontier",
		"clusterName", clusterName,
		"requested", firstRequiredWAL,
		"tier2Frontier", latestUploadedWAL,
		"clamped", clamped,
	)

	return clamped, nil
}

// clampWAL decides which WAL file should become the first required one given
// the WAL requested by the retention policy and the tier2 transfer frontier
// (the latest WAL known to have been uploaded to tier2).
//
//   - An empty frontier means nothing has been transferred yet, so we must not
//     delete anything and return "".
//   - When the frontier is older than the requested WAL, we clamp to the
//     frontier so WALs still pending transfer to tier2 are preserved.
//   - Otherwise the requested WAL is safe to apply as-is.
func clampWAL(requested, frontier string) string {
	if frontier == "" {
		return ""
	}

	if strings.Compare(frontier, requested) < 0 {
		return frontier
	}

	return requested
}

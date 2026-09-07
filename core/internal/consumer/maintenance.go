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
	"fmt"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
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

	// Delete the tier1 base backups that fall outside the retention policy,
	// while never deleting one that has not yet reached tier2, so no base backup
	// is lost before it is durable on tier2.
	keep, err := d.tier1RetentionGuard(ctx, clusterName)
	if err != nil {
		return err
	}

	if err := d.applyRetention(ctx, d.tier1Client, clusterName, task.Tier1RetentionPolicy, keep); err != nil {
		return fmt.Errorf("while applying tier1 retention policy: %w", err)
	}

	return d.applyTier1WALRetention(ctx, clusterName)
}

// tier1RetentionGuard returns a predicate that reports whether a tier1 backup
// must be kept because it has not yet been migrated to tier2. When tier2 is not
// configured there is nothing to protect and the predicate is nil.
func (d *Backup) tier1RetentionGuard(ctx context.Context, clusterName string) (func(name string) bool, error) {
	if !d.tier2Enabled {
		return nil, nil
	}

	tier2Backups, err := d.tier2Client.ListBackups(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf(
			"while listing tier2 backups to guard tier1 retention for cluster %q: %w", clusterName, err)
	}

	return keepUntilOnTier2(tier2Backups), nil
}

// keepUntilOnTier2 returns a predicate that reports whether a tier1 backup must
// be kept because it is not yet present on tier2. A backup becomes deletable
// once it appears in the tier2 catalog.
func keepUntilOnTier2(tier2Backups klioclient.BackupList) func(name string) bool {
	onTier2 := stringset.New()
	for i := range tier2Backups {
		onTier2.Put(tier2Backups[i].Name)
	}

	return func(name string) bool {
		return !onTier2.Has(name)
	}
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

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

package retention

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/policy"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
)

// ErrInvalidRetentionLatest is returned when a stored retention policy asks to
// keep a negative number of backups.
var ErrInvalidRetentionLatest = errors.New("invalid retention policy: latest must be greater than or equal to 0")

// backupDeleter is the subset of klioclient.Client that sweepCluster needs.
type backupDeleter interface {
	ListBackups(ctx context.Context, hostname string) (klioclient.BackupList, error)
	DeleteBackup(ctx context.Context, hostname string, name string) error
}

// tierPolicySelector picks this tier's TierRetentionPolicy out of the
// combined RetentionPolicy blob stored for a cluster.
type tierPolicySelector func(*grpc.RetentionPolicy) *grpc.TierRetentionPolicy

// tierConfig gathers everything sweepTier needs to sweep one tier, so
// tier1 and tier2 (identical logic, different clients/config) share one
// implementation instead of two near-duplicate functions.
type tierConfig struct {
	name            string
	metricTier      opentelemetry.Tier
	kopiaClient     *kopia.Client
	backupClient    backupDeleter
	selectPolicy    tierPolicySelector
	serverAddress   string
	certFingerprint string
	walRepository   *repository.Connection

	// syncGuardClient, when set, is queried before deleting a candidate
	// backup: the candidate is only deleted if it already exists there.
	// Only ever set on tier1's config (pointing at tier2), to stop tier1
	// retention from deleting a backup tier2 hasn't received yet. Left nil
	// when there's no further tier to protect against.
	syncGuardClient backupDeleter
}

// Sweep evaluates and deletes out-of-retention backups for tier1 (and,
// when configured, tier2), across every cluster in the repository.
// Failures are per-tier and best-effort: one tier's error does not stop the
// other from being swept. All the errors are joined and returned.
func (s *Sweeper) Sweep(ctx context.Context) error {
	var errs []error

	var tier1SyncGuard backupDeleter
	if s.tier2Enabled {
		tier1SyncGuard = s.tier2Client
	}

	if err := s.sweepTier(ctx, tierConfig{
		name:            "tier1",
		metricTier:      opentelemetry.Tier1,
		kopiaClient:     s.tier1Kopia,
		backupClient:    s.tier1Client,
		selectPolicy:    func(p *grpc.RetentionPolicy) *grpc.TierRetentionPolicy { return p.GetTier1Policy() },
		serverAddress:   s.opts.Tier1ServerAddress,
		certFingerprint: s.opts.Tier1ServerCertificateFingerprint,
		walRepository:   s.opts.Tier1WALRepository,
		syncGuardClient: tier1SyncGuard,
	}); err != nil {
		errs = append(errs, fmt.Errorf("while sweeping tier1 retention: %w", err))
	}

	if s.tier2Enabled {
		if err := s.sweepTier(ctx, tierConfig{
			name:            "tier2",
			metricTier:      opentelemetry.Tier2,
			kopiaClient:     s.tier2Kopia,
			backupClient:    s.tier2Client,
			selectPolicy:    func(p *grpc.RetentionPolicy) *grpc.TierRetentionPolicy { return p.GetTier2Policy() },
			serverAddress:   s.opts.Tier2ServerAddress,
			certFingerprint: s.opts.Tier2ServerCertificateFingerprint,
			walRepository:   s.opts.Tier2WALRepository,
		}); err != nil {
			errs = append(errs, fmt.Errorf("while sweeping tier2 retention: %w", err))
		}
	}

	return errors.Join(errs...)
}

// sweepTier sweeps every cluster on one tier, refreshing that tier's Kopia
// server cache once at the end if, and only if, something was deleted. A
// failure on one cluster does not stop the others; all the failures are
// logged with their context and returned joined.
func (s *Sweeper) sweepTier(ctx context.Context, tc tierConfig) error {
	contextLogger := log.FromContext(ctx)

	hostnames, err := listClusterHostnames(ctx, tc.kopiaClient)
	if err != nil {
		return fmt.Errorf("while listing %s clusters: %w", tc.name, err)
	}

	var errs []error

	deletedAny := false
	for _, hostname := range hostnames {
		deleted, retentionErr := s.sweepCluster(ctx, hostname, tc)
		if retentionErr != nil {
			contextLogger.Error(retentionErr, "Error while sweeping retention for cluster",
				"tier", tc.name, "cluster", hostname)
			errs = append(errs, fmt.Errorf("cluster %q: %w", hostname, retentionErr))
		}

		// Deletions that succeeded must be reflected in the server cache even
		// when other deletions of the same cluster failed.
		deletedAny = deletedAny || deleted > 0

		walErr := s.applyWALRetention(ctx, hostname, tc)
		if walErr != nil {
			contextLogger.Error(walErr, "Error while applying WAL retention", "tier", tc.name, "cluster", hostname)
			errs = append(errs, fmt.Errorf("cluster %q WAL retention: %w", hostname, walErr))
		}

		recordMaintenance(ctx, hostname, tc.metricTier, errors.Join(retentionErr, walErr))
	}

	orphansDeleted, err := s.sweepOrphans(ctx, tc, hostnames)
	if err != nil {
		contextLogger.Error(err, "Error while sweeping orphan snapshots", "tier", tc.name)
		errs = append(errs, fmt.Errorf("orphan snapshots: %w", err))
	}
	deletedAny = deletedAny || orphansDeleted > 0

	if deletedAny {
		contextLogger.Info("Refreshing Kopia server cache after retention sweep", "tier", tc.name)

		if err := tc.kopiaClient.RefreshServer(ctx, kopia.RefreshServerOptions{
			ServerControlUser:     s.opts.RunID,
			ServerControlPassword: s.opts.RunSecret,
			ServerCertFingerprint: tc.certFingerprint,
			Address:               tc.serverAddress,
		}); err != nil {
			errs = append(errs, fmt.Errorf("while refreshing the %s Kopia server: %w", tc.name, err))
		}
	}

	return errors.Join(errs...)
}

// sweepCluster deletes, for one cluster on one tier, whatever backups the
// configured policy decides are out of retention, and returns how many it
// deleted.
func (s *Sweeper) sweepCluster(
	ctx context.Context,
	clusterName string,
	tc tierConfig,
) (int, error) {
	contextLogger := log.FromContext(ctx)

	retentionPolicy, err := walserver.GetClusterRetentionPolicy(ctx, s.opts.Tier1WALRepository, clusterName)
	if err != nil {
		return 0, fmt.Errorf("while reading retention policy: %w", err)
	}

	tierPolicy := tc.selectPolicy(retentionPolicy)
	if tierPolicy == nil {
		// No policy configured yet for this cluster: do nothing. This happens
		// on every pass, so it is not logged above debug level.
		contextLogger.Debug("No retention policy configured for cluster, skipping",
			"tier", tc.name, "cluster", clusterName)

		return 0, nil
	}

	evaluator, err := policyFromProto(tierPolicy)
	if err != nil {
		return 0, err
	}

	backups, err := tc.backupClient.ListBackups(ctx, clusterName)
	if err != nil {
		return 0, fmt.Errorf("while listing backups: %w", err)
	}
	if len(backups) == 0 {
		return 0, nil
	}

	toDelete, err := deletionCandidates(ctx, clusterName, tc, retentionPolicy, evaluator, backups)
	if err != nil {
		return 0, err
	}

	if len(toDelete) == 0 {
		contextLogger.Debug("No backup is out of retention",
			"tier", tc.name, "cluster", clusterName, "latest", tierPolicy.GetLatest(), "backups", len(backups))

		return 0, nil
	}

	targets := make([]hostBackupName, 0, len(toDelete))
	for _, b := range toDelete {
		targets = append(targets, hostBackupName{Host: clusterName, Name: b.Name})
	}

	contextLogger.Info("Deleting out-of-retention backups",
		"tier", tc.name, "cluster", clusterName, "latest", tierPolicy.GetLatest(),
		"backups", len(backups), "toDelete", backupNames(targets))

	deleted, err := deleteBackups(ctx, tc.backupClient, targets)
	contextLogger.Info("Out-of-retention backups deleted",
		"tier", tc.name, "cluster", clusterName, "deleted", deleted, "failed", len(targets)-deleted)

	return deleted, err
}

// deleteBackups deletes every target and returns how many were deleted. A
// failure does not stop the remaining deletions: the errors of all the
// failed ones are joined and returned.
func deleteBackups(ctx context.Context, deleter backupDeleter, targets []hostBackupName) (int, error) {
	contextLogger := log.FromContext(ctx)

	var errs []error

	deleted := 0
	for _, t := range targets {
		if err := deleter.DeleteBackup(ctx, t.Host, t.Name); err != nil {
			contextLogger.Error(err, "Error while deleting backup", "cluster", t.Host, "backup", t.Name)
			errs = append(errs, fmt.Errorf("while deleting backup %q of cluster %q: %w", t.Name, t.Host, err))

			continue
		}

		deleted++
	}

	return deleted, errors.Join(errs...)
}

// backupNames returns the "cluster/backup" names of targets, for logging.
func backupNames(targets []hostBackupName) []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.Host+"/"+t.Name)
	}

	return names
}

// deletionCandidates computes which of backups are out of retention
// (evaluator decides what to keep) and, when tc guards tier1 against
// unsynced tier2 backups, filters that set down to only the ones already
// present on tier2.
func deletionCandidates(
	ctx context.Context,
	clusterName string,
	tc tierConfig,
	retentionPolicy *grpc.RetentionPolicy,
	evaluator policy.Policy,
	backups klioclient.BackupList,
) (klioclient.BackupList, error) {
	// Evaluate sorts and slices in place; operate on a copy so the caller's
	// slice (and its original order) is left untouched.
	keep := evaluator.Evaluate(append(klioclient.BackupList{}, backups...))
	toDelete := diffByName(backups, keep)

	if tc.syncGuardClient == nil || retentionPolicy.GetTier2Policy() == nil {
		return toDelete, nil
	}

	guarded, err := protectUnsyncedToTier2(ctx, clusterName, toDelete, tc.syncGuardClient)
	if err != nil {
		return nil, fmt.Errorf("while checking tier2 sync status: %w", err)
	}

	if len(guarded) < len(toDelete) {
		log.FromContext(ctx).Info(
			"Some out-of-retention backups are not yet synced to tier2, keeping them on tier1 for now",
			"tier", tc.name, "cluster", clusterName,
			"outOfRetention", len(toDelete), "protected", len(toDelete)-len(guarded),
		)
	}

	return guarded, nil
}

// protectUnsyncedToTier2 filters candidates down to only the backups
// already present on tier2 (via tier2Client), so tier1 retention never
// deletes a backup tier2 hasn't received yet. On a tier2-listing error, the
// caller treats this as "delete nothing this tick" rather than guessing.
func protectUnsyncedToTier2(
	ctx context.Context,
	clusterName string,
	candidates klioclient.BackupList,
	tier2Client backupDeleter,
) (klioclient.BackupList, error) {
	tier2Backups, err := tier2Client.ListBackups(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("while listing tier2 backups: %w", err)
	}

	onTier2 := make(map[string]struct{}, len(tier2Backups))
	for _, b := range tier2Backups {
		onTier2[b.Name] = struct{}{}
	}

	var safe klioclient.BackupList
	for _, b := range candidates {
		if _, ok := onTier2[b.Name]; ok {
			safe = append(safe, b)
		}
	}

	return safe, nil
}

// policyFromProto builds the policy.Policy that implements a cluster's
// stored TierRetentionPolicy. The only strategy today is keep-latest-N; a
// future retention shape (e.g. time-based) would add a case here rather
// than changing anything else in the sweep.
//
//nolint:ireturn // policy.Policy is the deliberate extension point described above.
func policyFromProto(p *grpc.TierRetentionPolicy) (policy.Policy, error) {
	latest := int(p.GetLatest())
	if latest < 0 {
		return nil, fmt.Errorf("%w: got %d", ErrInvalidRetentionLatest, latest)
	}

	return &policy.LatestPolicy{Count: latest}, nil
}

// diffByName returns the entries of all that are not present (by Name) in
// keep -- the backups a Policy.Evaluate call decided to drop.
func diffByName(all, keep klioclient.BackupList) klioclient.BackupList {
	keepNames := make(map[string]struct{}, len(keep))
	for _, b := range keep {
		keepNames[b.Name] = struct{}{}
	}

	var dropped klioclient.BackupList
	for _, b := range all {
		if _, ok := keepNames[b.Name]; !ok {
			dropped = append(dropped, b)
		}
	}

	return dropped
}

// listClusterHostnames returns one hostname per distinct cluster with a
// backup on client, derived from the backup metadata snapshots.
func listClusterHostnames(ctx context.Context, client *kopia.Client) ([]string, error) {
	contextLogger := log.FromContext(ctx)

	entries, err := client.ListSnapshots(ctx, map[string]string{
		klioclient.BackupContentTagName: "metadata",
	}, contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	return groupHostnames(entries), nil
}

// groupHostnames reduces a manifest list to its distinct source hostnames,
// split out of listClusterHostnames so the grouping logic can be unit
// tested without a real Kopia client.
func groupHostnames(entries []kopia.Manifest) []string {
	seen := make(map[string]struct{}, len(entries))
	hostnames := make([]string, 0, len(entries))

	for _, entry := range entries {
		if _, ok := seen[entry.Source.Host]; ok {
			continue
		}
		seen[entry.Source.Host] = struct{}{}
		hostnames = append(hostnames, entry.Source.Host)
	}

	return hostnames
}

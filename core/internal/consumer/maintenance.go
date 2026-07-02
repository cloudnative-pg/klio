package consumer

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// maintainTier1 applies the tier1 retention policy and drops the WAL files
// that are no longer required by any remaining tier1 backup. It records the
// tier1 maintenance metric; the error is returned for logging but is
// best-effort (the caller does not fail the task on it).
//
// This is the server-side equivalent of the work that the `klio backup
// maintenance` command used to perform client-side after every backup.
func (d *Backup) maintainTier1(ctx context.Context, clusterName string, entries []kopia.Manifest) error {
	if len(entries) == 0 {
		return nil
	}

	log.FromContext(ctx).Info("Applying tier1 maintenance", "cluster", clusterName)

	err := d.runTier1Retention(ctx, clusterName, entries)
	recordMaintenance(ctx, clusterName, opentelemetry.Tier1, err)

	return err
}

func (d *Backup) runTier1Retention(ctx context.Context, clusterName string, entries []kopia.Manifest) error {
	// The cluster name reaches us from the client's CloseBackup request via the
	// queue task and is used below as a WAL directory path, so validate it
	// before we touch the filesystem. This guard used to live in the gRPC
	// SetFirstRequiredWAL handler that drove retention before it moved here.
	if err := repository.ValidatePathComponent(clusterName); err != nil {
		return fmt.Errorf("invalid cluster name %q: %w", clusterName, err)
	}

	userName := entries[0].Source.UserName

	// Apply the tier1 retention policy, deleting any base snapshots that are
	// no longer needed.
	if err := d.tier1Client.ApplyRetentionPolicy(ctx, kopia.Target{
		Username: userName,
		Hostname: clusterName,
	}); err != nil {
		return fmt.Errorf("while applying tier1 retention policy: %w", err)
	}

	return d.applyTier1WALRetention(ctx, clusterName)
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

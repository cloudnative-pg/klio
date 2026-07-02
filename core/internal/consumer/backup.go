package consumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// errTier2NotConfigured is returned when a backup requests a tier2 relay but
// the server has no tier2 configured. It fails the task (retried, then
// dead-lettered) so the misconfiguration is surfaced.
var errTier2NotConfigured = errors.New("backup requested tier2 relay but the server has no tier2 configured")

// backupSteps is implemented by *Backup in production and by a test stub in
// unit tests. It covers the five steps that processBackup orchestrates so
// that the orchestration logic can be exercised without real Kopia clients.
type backupSteps interface {
	listManifests(ctx context.Context, clusterName string) ([]kopia.Manifest, error)
	verifyTier1(ctx context.Context, clusterName string) error
	relayTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error
	maintainTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error
	maintainTier1(ctx context.Context, clusterName string, entries []kopia.Manifest) error
}

// Backup represents a Backup consumer.
type Backup struct {
	opts         *BackupOptions
	tier1Kopia   *kopia.Client
	tier2Kopia   *kopia.Client
	tier1Client  *klioclientkopia.Connection
	tier2Client  *klioclientkopia.Connection
	tier2Enabled bool
	steps        backupSteps
}

// BackupOptions are the configuration of the WAL consumer.
type BackupOptions struct {
	// The queue to be used
	Queue *queue.Conn

	// A config file to connect to tier 1
	Tier1KopiaConfig string

	// A config file to connect to tier 2
	Tier2KopiaConfig string

	// The cache directory
	CacheDirectory string

	// RunID is the unique identifier for this server run.
	RunID string

	// RunSecret is the secret credential for server control operations.
	RunSecret string

	// Tier2ServerAddress is the address of the tier 2 Kopia server.
	Tier2ServerAddress string

	// Tier2ServerCertificateFingerprint is the SHA256 fingerprint of the tier 2 server certificate.
	Tier2ServerCertificateFingerprint string

	// Tier2WALRepository is the connection to the tier 2 WAL repository.
	// Used to apply WAL retention after backup retention is applied.
	Tier2WALRepository *repository.Connection

	// Tier1WALRepository is the connection to the tier 1 WAL repository.
	// Used by tier1 maintenance to drop WAL files that are no longer required.
	Tier1WALRepository *repository.Connection
}

// NewBackup creates a new Backup consumer.
func NewBackup(opts *BackupOptions) (*Backup, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	tier1Client, err := klioclientkopia.FromKopiaConfig(opts.Tier1KopiaConfig)
	if err != nil {
		return nil, fmt.Errorf("while creating tier1 client: %w", err)
	}

	b := &Backup{
		opts: opts,
		tier1Kopia: &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier1KopiaConfig,
		},
		tier1Client: tier1Client,
	}

	// The tier2 clients are only created when tier2 is configured. Without
	// them the consumer only performs tier1 maintenance.
	if opts.Tier2KopiaConfig != "" {
		tier2Client, err := klioclientkopia.FromKopiaConfig(opts.Tier2KopiaConfig)
		if err != nil {
			return nil, fmt.Errorf("while creating tier2 client: %w", err)
		}

		b.tier2Kopia = &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier2KopiaConfig,
		}
		b.tier2Client = tier2Client
		b.tier2Enabled = true
	}

	b.steps = b

	return b, nil
}

// Run starts the consumer until the context is canceled or the
// SIGINT signal arrives.
func (d *Backup) Run(ctx context.Context) error {
	consumerCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return d.opts.Queue.ConsumeBackupReceivedMessages(consumerCtx, d.processBackup)
}

// processBackup performs the post-backup work for a task and records a
// per-operation metric for the relay and maintenance stages. A returned error
// means the task should be retried (and, once MaxDeliver is exhausted,
// dead-lettered).
func (d *Backup) processBackup(ctx context.Context, task *queue.BackupTask) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Processing backup", "task", task)

	entries, err := d.steps.listManifests(ctx, task.ClusterName)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	// Verify tier1 backups (catches any issues since sidecar verification).
	// We verify all backups for the cluster rather than a specific one because
	// MigrateSnapshots works at the source level (cluster), not individual backups.
	// The BackupTask doesn't include the backup name for this reason.
	if err := d.steps.verifyTier1(ctx, task.ClusterName); err != nil {
		return err
	}

	return d.relayAndMaintain(ctx, task, entries)
}

// relayAndMaintain runs the tier2 relay (when requested) and the per-tier
// maintenance for a verified backup.
func (d *Backup) relayAndMaintain(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error {
	contextLogger := log.FromContext(ctx)

	// A tier2 relay requested against a server with no tier2 is a
	// misconfiguration: we record it as a relay failure, still run tier1
	// maintenance below (so tier1 retention is never starved), then fail the
	// task so it is retried and eventually dead-lettered, surfacing the gap.
	tier2Unavailable := task.SendToTier2 && !d.tier2Enabled
	switch {
	case tier2Unavailable:
		contextLogger.Error(nil,
			"Backup requested tier2 relay but the server has no tier2 configured",
			"cluster", task.ClusterName)
		recordRelay(ctx, task.ClusterName, errTier2NotConfigured)
	case task.SendToTier2:
		relayErr := d.steps.relayTier2(ctx, task, entries)
		recordRelay(ctx, task.ClusterName, relayErr)
		if relayErr != nil {
			return relayErr
		}

		// tier2 maintenance (retention + WAL cleanup) records its own per-tier
		// metric; a tier2 base-retention failure is fatal (the task is retried)
		// while WAL cleanup is best-effort.
		if err := d.steps.maintainTier2(ctx, task, entries); err != nil {
			return err
		}
	}

	// Tier1 maintenance: apply the tier1 retention policy and drop WAL files
	// that are no longer required. This runs for every backup, including the
	// misconfigured one above. It records its own per-tier metric (the only
	// signal of a tier1 maintenance failure, which is otherwise best-effort);
	// we log but don't fail the task on its error.
	if err := d.steps.maintainTier1(ctx, task.ClusterName, entries); err != nil {
		contextLogger.Error(err, "Error while applying tier1 maintenance, skipping")
	}

	if tier2Unavailable {
		return errTier2NotConfigured
	}

	return nil
}

// relayTier2 migrates the cluster's backups to tier2 and verifies them there.
// tier2 retention/WAL cleanup is handled separately by maintainTier2.
func (d *Backup) relayTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error {
	sources := manifestListToDescriptors(entries)

	if err := d.tier2Kopia.MigrateSnapshots(ctx, kopia.SnapshotMigrateOpts{
		SourceConfig: d.opts.Tier1KopiaConfig,
		Sources:      sources,
		Tags: []string{
			klioclient.TablespaceNameTagName,
			klioclient.BackupContentTagName,
			klioclient.BackupNameTagName,
		},
	}); err != nil {
		return err
	}

	// Verify tier2 backups after migration
	return d.verifyTier2Backups(ctx, task.ClusterName)
}

// maintainTier2 enforces tier2 retention (base-snapshot policy and WAL
// cleanup) after a successful relay, and records the tier2 maintenance metric.
// A base-retention failure (policy set/apply) is fatal so the task is retried;
// unpin, server refresh and WAL cleanup are best-effort. The ordering matches
// the previous inline flow: WAL retention runs after the server refresh so it
// lists the post-retention backups.
func (d *Backup) maintainTier2(ctx context.Context, task *queue.BackupTask, entries []kopia.Manifest) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Applying tier2 maintenance", "cluster", task.ClusterName)

	target := kopia.Target{
		Username: entries[0].Source.UserName,
		Hostname: task.ClusterName,
	}

	if task.Tier2RetentionPolicy != nil {
		if err := d.tier2Kopia.SetKopiaPolicy(ctx, target, task.Tier2RetentionPolicy); err != nil {
			recordMaintenance(ctx, task.ClusterName, opentelemetry.Tier2, err)

			return err
		}
	}

	if err := d.tier2Kopia.ApplyKopiaPolicy(ctx, target); err != nil {
		recordMaintenance(ctx, task.ClusterName, opentelemetry.Tier2, err)

		return err
	}

	// Unpin the pinned snapshots (best-effort: the backup is already on tier2;
	// they will be unpinned when migrating the next backup).
	if pinnedSnapshots := getPinnedSnapshots(entries); len(pinnedSnapshots) > 0 {
		if err := d.tier1Kopia.PinSnapshots(ctx, kopia.PinSnapshotOpts{
			IDs:        pinnedSnapshots,
			RemovePins: []string{klioclient.Tier2Pin},
		}); err != nil {
			contextLogger.Error(err, "Error while unpinning snapshots")
		}
	}

	// Refresh the tier 2 server cache so it reflects the post-retention manifest
	// list before WAL retention lists the surviving backups (best-effort).
	if err := d.refreshTier2KopiaServer(ctx); err != nil {
		contextLogger.Error(err, "Error while refreshing Kopia server cache, skipping")
	}

	// Apply WAL retention to tier2 based on the remaining backups (best-effort:
	// recorded on the maintenance metric, but does not fail the task).
	var walErr error
	if d.opts.Tier2WALRepository != nil {
		if walErr = d.applyTier2WALRetention(ctx, task.ClusterName); walErr != nil {
			contextLogger.Error(walErr, "Error while applying tier2 WAL retention, skipping")
		}
	}

	recordMaintenance(ctx, task.ClusterName, opentelemetry.Tier2, walErr)

	return nil
}

func (d *Backup) listManifests(ctx context.Context, cluster string) ([]kopia.Manifest, error) {
	contextLogger := log.FromContext(ctx)
	entries, err := d.tier1Kopia.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	result := make([]kopia.Manifest, 0, len(entries))
	for i := range entries {
		if entries[i].Source.Host != cluster {
			continue
		}

		result = append(result, entries[i])
	}

	return result, nil
}

func manifestListToDescriptors(entries []kopia.Manifest) []string {
	result := stringset.New()
	for _, entry := range entries {
		result.Put(entry.Source.String())
	}

	return result.ToSortedList()
}

func getPinnedSnapshots(manifests []kopia.Manifest) []string {
	result := stringset.New()

	for i := range manifests {
		if len(manifests[i].Pins) > 0 && manifests[i].RootEntry != nil && manifests[i].RootEntry.ObjID != "" {
			result.Put(manifests[i].RootEntry.ObjID)
		}
	}

	return result.ToSortedList()
}

// refreshTier2KopiaServer makes sure the tier 2 kopia server
// has downloaded the latest manifests from the object store.
func (d *Backup) refreshTier2KopiaServer(ctx context.Context) error {
	return d.tier2Kopia.RefreshServer(ctx, kopia.RefreshServerOptions{
		ServerControlUser:     d.opts.RunID,
		ServerControlPassword: d.opts.RunSecret,
		ServerCertFingerprint: d.opts.Tier2ServerCertificateFingerprint,
		Address:               d.opts.Tier2ServerAddress,
	})
}

// applyTier2WALRetention applies WAL retention to tier2 based on the remaining backups.
// After Kopia retention policy is applied (which may delete old backups), this function
// finds the oldest remaining backup and removes WAL files that are no longer needed.
func (d *Backup) applyTier2WALRetention(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)

	// List remaining backups in tier2 for this cluster
	backups, err := d.tier2Client.ListBackups(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("while listing tier2 backups for cluster %q: %w", clusterName, err)
	}

	// Early exit if no backups exist
	if len(backups) == 0 {
		contextLogger.Info("No backups found in tier2; skipping WAL retention")
		return nil
	}

	oldestWAL := findOldestWAL(backups)

	// No oldest WAL found, nothing to do
	if oldestWAL == "" {
		contextLogger.Info("Backups exist but none contain a StartWAL; skipping WAL retention")
		return nil
	}

	contextLogger.Info("Applying tier2 WAL retention", "clusterName", clusterName, "oldestWAL", oldestWAL)

	// Apply WAL retention
	if err := d.opts.Tier2WALRepository.SetFirstRequiredOnCluster(ctx, clusterName, oldestWAL); err != nil {
		return fmt.Errorf("while applying WAL retention on tier2: %w", err)
	}

	return nil
}

func (d *Backup) verifyTier1(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Verifying tier1 backups", "cluster", clusterName)

	err := d.tier1Client.VerifyBackups(ctx, klioclientkopia.VerifyOpts{
		Hostname: clusterName,
		All:      true,
	})
	if err != nil {
		if _, ok := errors.AsType[*klioclientkopia.BackupVerificationError](err); ok {
			recordVerificationFailure(ctx, opentelemetry.Tier1)

			return fmt.Errorf("tier1 verification detected corruption: %w", err)
		}
		contextLogger.Error(err, "Tier1 verification encountered infrastructure error")

		return err
	}

	recordVerificationSuccess(ctx, opentelemetry.Tier1)

	return nil
}

func (d *Backup) verifyTier2Backups(ctx context.Context, clusterName string) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Verifying tier2 backups after migration", "cluster", clusterName)

	err := d.tier2Client.VerifyBackups(ctx, klioclientkopia.VerifyOpts{
		Hostname: clusterName,
		All:      true,
	})
	if err != nil {
		if _, ok := errors.AsType[*klioclientkopia.BackupVerificationError](err); ok {
			recordVerificationFailure(ctx, opentelemetry.Tier2)

			return fmt.Errorf("tier2 verification detected corruption: %w", err)
		}
		contextLogger.Error(err, "Tier2 verification encountered infrastructure error, continuing")
	} else {
		recordVerificationSuccess(ctx, opentelemetry.Tier2)
	}

	return nil
}

func findOldestWAL(backups klioclient.BackupList) string {
	var oldestWAL string

	for _, b := range backups {
		if b.StartWAL == "" {
			continue
		}
		// If oldestWAL is empty, or the current backup's WAL is lexicographically smaller
		if oldestWAL == "" || b.StartWAL < oldestWAL {
			oldestWAL = b.StartWAL
		}
	}

	return oldestWAL
}

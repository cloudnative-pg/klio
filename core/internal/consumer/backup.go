package consumer

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	klioclientkopia "github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Backup represents a Backup consumer.
type Backup struct {
	opts        *BackupOptions
	tier1Kopia  *kopia.Client
	tier2Kopia  *kopia.Client
	tier2Client *klioclientkopia.Connection
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
}

// NewBackup creates a new Backup consumer.
func NewBackup(opts *BackupOptions) (*Backup, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	tier2Client, err := klioclientkopia.FromKopiaConfig(opts.Tier2KopiaConfig)
	if err != nil {
		return nil, fmt.Errorf("while creating tier2 client: %w", err)
	}

	return &Backup{
		opts: opts,
		tier1Kopia: &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier1KopiaConfig,
		},
		tier2Kopia: &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  opts.Tier2KopiaConfig,
		},
		tier2Client: tier2Client,
	}, nil
}

// Run starts the consumer until the context is canceled or the
// SIGINT signal arrives.
func (d *Backup) Run(ctx context.Context) error {
	consumerCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return d.opts.Queue.ConsumeBackupReceivedMessages(consumerCtx, d.backupHandler)
}

func (d *Backup) backupHandler(ctx context.Context, task *queue.BackupTask) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Synchronizing backup", "task", task)

	entries, err := d.sourcesForCluster(ctx, task.ClusterName)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	sources := sourceInfoListToDescriptors(entries)

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

	userName := entries[0].UserName

	if task.Tier2RetentionPolicy != nil {
		if err := d.tier2Kopia.SetKopiaPolicy(
			ctx,
			kopia.Target{
				Username: userName,
				Hostname: task.ClusterName,
			},
			task.Tier2RetentionPolicy,
		); err != nil {
			return err
		}
	}

	if err := d.tier2Kopia.ApplyKopiaPolicy(
		ctx,
		kopia.Target{
			Username: userName,
			Hostname: task.ClusterName,
		},
	); err != nil {
		return err
	}

	// Refresh the tier 2 server cache to ensure it has the latest manifests.
	// After applying retention policies, the manifest list may have changed,
	// so we need to notify the Kopia server to update its cache.
	// Note: We log but don't fail on this error because the backup migration
	// has already succeeded. Failing here would cause unnecessary retries of the
	// entire migration, which is wasteful since the data is already safe.
	if err := d.refreshTier2KopiaServer(ctx); err != nil {
		contextLogger.Error(err, "Error while refreshing Kopia server cache, skipping")
	}

	// Apply WAL retention to tier2 based on remaining backups.
	// This ensures tier2 WAL files are cleaned up according to tier2's own retention policy,
	// not tier1's policy.
	if d.opts.Tier2WALRepository != nil {
		if err := d.applyTier2WALRetention(ctx, task.ClusterName); err != nil {
			contextLogger.Error(err, "Error while applying tier2 WAL retention, skipping")
		}
	}

	return nil
}

// ListBackups list all Kopia sources for a specified cluster.
func (d *Backup) sourcesForCluster(ctx context.Context, cluster string) ([]kopia.SourceInfo, error) {
	contextLogger := log.FromContext(ctx)
	entries, err := d.tier1Kopia.ListSnapshots(ctx, nil, contextLogger.Info)
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	result := make([]kopia.SourceInfo, 0, len(entries))
	for i := range entries {
		if entries[i].Source.Host != cluster {
			continue
		}

		result = append(result, entries[i].Source)
	}

	return result, nil
}

func sourceInfoListToDescriptors(entries []kopia.SourceInfo) []string {
	result := stringset.New()
	for _, entry := range entries {
		result.Put(entry.String())
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

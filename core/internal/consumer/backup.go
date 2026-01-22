package consumer

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// Backup represents a Backup consumer.
type Backup struct {
	opts       *BackupOptions
	tier1Kopia *kopia.Client
	tier2Kopia *kopia.Client
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
}

// NewBackup creates a new Backup consumer.
func NewBackup(opts *BackupOptions) (*Backup, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
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
	// Note: We log but don't fail on refresh errors because the backup migration
	// has already succeeded. Failing here would cause unnecessary retries of the
	// entire migration, which is wasteful since the data is already safe.
	if err := d.refreshTier2KopiaServer(ctx); err != nil {
		contextLogger.Error(err, "Error while refreshing Kopia server cache, skipping")
	}

	return nil
}

// ListBackups list all Kopia sources for a specified cluster.
func (d *Backup) sourcesForCluster(ctx context.Context, cluster string) ([]kopia.SourceInfo, error) {
	entries, err := d.tier1Kopia.ListSnapshots(ctx, nil)
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

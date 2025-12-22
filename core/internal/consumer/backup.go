package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// Backup represents a Backup consumer.
type Backup struct {
	opts *BackupOptions

	kopiaBinary string
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

	// The tier1 encryption password (LEO: why?)
	Tier1EncryptionPassword string
}

// NewBackup creates a new WAL consumer.
func NewBackup(opts *BackupOptions) (*Backup, error) {
	kopiaBinary, err := exec.LookPath(klioclient.KopiaCommand)
	if err != nil {
		return nil, fmt.Errorf("kopia binary not found (%q): %w", klioclient.KopiaCommand, err)
	}

	return &Backup{
		opts:        opts,
		kopiaBinary: kopiaBinary,
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

	log.Info("Synchronizing backup", "task", task)

	sources, err := d.sourcesForCluster(ctx, task.ClusterName)
	if err != nil {
		return err
	}

	// Migrate the Kopia Snapshots
	args := []string{
		"snapshot", "migrate",
		"--source-config=" + d.opts.Tier1KopiaConfig,
		"--config-file=" + d.opts.Tier2KopiaConfig,
		"--disable-file-logging",
		"--json-log-console",
		"--tags=tag:" + klioclient.TablespaceNameTagName,
		"--tags=tag:" + klioclient.BackupContentTagName,
		"--tags=tag:" + klioclient.BackupNameTagName,
	}

	for _, source := range sources {
		args = append(args, "--sources="+source)
	}

	kopiaMigrate := exec.CommandContext(ctx, d.kopiaBinary, args...) //nolint:gosec
	kopiaMigrate.Stdout = os.Stdout
	kopiaMigrate.Stderr = os.Stderr

	contextLogger.Info("Starting Kopia migration", "args", kopiaMigrate.Args)

	if err := kopiaMigrate.Start(); err != nil {
		return fmt.Errorf("while starting the kopia migration: %w", err)
	}

	if err := kopiaMigrate.Wait(); err != nil {
		return fmt.Errorf("while running the kopia migration: %w", err)
	}

	return nil
}

// ListBackups list all Kopia sources for a specified cluster.
func (d *Backup) sourcesForCluster(ctx context.Context, cluster string) ([]string, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--config-file=" + d.opts.Tier1KopiaConfig,
	}

	var stdout bytes.Buffer
	snapshotList := exec.CommandContext(ctx, d.kopiaBinary, args...) //nolint:gosec
	snapshotList.Stdout = &stdout
	snapshotList.Stderr = os.Stderr

	contextLogger.Info("Looking for Kopia sources for cluster", "args", snapshotList.Args, "cluster", cluster)
	if err := snapshotList.Run(); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []klioclient.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	result := stringset.New()
	for _, entry := range entries {
		if entry.Source.Host != cluster {
			continue
		}

		result.Put(entry.Source.String())
	}

	return result.ToSortedList(), nil
}

package consumer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/queue"
)

// kopiaCommand is the name of the kopia binary.
const kopiaCommand = "kopia"

// Backup represents a Backup consumer.
type Backup struct {
	opts *BackupOptions
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
func NewBackup(opts *BackupOptions) *Backup {
	return &Backup{
		opts: opts,
	}
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

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	// Start the Kopia server
	args := []string{
		"snapshot", "migrate",
		"--source-config=" + d.opts.Tier1KopiaConfig,
		"--config-file=" + d.opts.Tier2KopiaConfig,
		"--disable-file-logging",
		"--json-log-console",
	}

	for _, source := range task.Sources {
		args = append(args, "--sources="+source)
	}

	kopiaMigrate := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaMigrate.Env = append(kopiaMigrate.Env,
		"KOPIA_LOG_DIR="+d.opts.CacheDirectory,
	)

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

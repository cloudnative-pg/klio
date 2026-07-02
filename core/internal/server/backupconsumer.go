package server

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/consumer"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// BackupConsumer processes backup tasks from the queue. It runs whenever tier1
// is enabled and performs the post-backup work for every backup: tier1
// maintenance always, plus the tier2 relay and tier2 maintenance when tier2 is
// configured. It is therefore not tier2-specific.
type BackupConsumer struct {
	Config               *config.ServerConfig
	Tier1KopiaConfigFile string
	Tier2KopiaConfigFile string
	QueueURL             string
	RunID                string
	RunSecret            string
}

// Serve starts the backup consumer and processes backup tasks from the queue.
func (s *BackupConsumer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("backup-consumer")
	ctx = log.IntoContext(ctx, contextLogger)

	// Connect to NATS
	natsConnection, err := nats.Connect(
		s.QueueURL,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("error while connecting to the NATS server: %w", err)
	}
	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		return fmt.Errorf("error while configuring NATS server: %w", err)
	}

	// Connect to the tier 1 WAL repository for tier1 maintenance (WAL retention)
	tier1WALFS := afero.NewBasePathFs(afero.NewOsFs(), s.Config.Tier1.Wal.WALPath)
	tier1WALRepository, err := repository.Open(repository.Options{
		FS:       tier1WALFS,
		Password: s.Config.Tier1.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to open tier1 WAL repository: %w", err)
	}

	backupOptions := &consumer.BackupOptions{
		Queue:              queueConnection,
		Tier1KopiaConfig:   s.Tier1KopiaConfigFile,
		CacheDirectory:     s.Config.Tier1.Base.CacheDirectory,
		RunID:              s.RunID,
		RunSecret:          s.RunSecret,
		Tier1WALRepository: tier1WALRepository,
	}

	// When tier2 is configured, wire the tier2 connections so the consumer
	// can relay backups destined for tier2. Without tier2 the consumer only
	// performs tier1 maintenance.
	if s.Tier2KopiaConfigFile != "" {
		// Extract the certificate fingerprint for the tier 2 Kopia server
		certificateFingerprint, err := kopia.ExtractSHA256CertificateFingerprint(
			s.Config.TLS.TLSCert)
		if err != nil {
			return fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
		}

		// Connect to the tier 2 WAL repository for WAL retention
		tier2WALFS, err := tier2.ConnectWAL(ctx, &s.Config.Tier2)
		if err != nil {
			return fmt.Errorf("error while connecting to tier2 WAL storage: %w", err)
		}
		tier2WALRepository, err := repository.Open(repository.Options{
			FS:       tier2WALFS,
			Password: s.Config.Tier2.EncryptionKey,
		})
		if err != nil {
			return fmt.Errorf("failed to open tier2 WAL repository: %w", err)
		}

		backupOptions.Tier2KopiaConfig = s.Tier2KopiaConfigFile
		backupOptions.Tier2ServerAddress = "https://" + s.Config.Tier2.BaseListenAddress
		backupOptions.Tier2ServerCertificateFingerprint = certificateFingerprint
		backupOptions.Tier2WALRepository = tier2WALRepository
	}

	// Starts the consumer
	c, err := consumer.NewBackup(backupOptions)
	if err != nil {
		return fmt.Errorf("error while creating backup consumer: %w", err)
	}

	if err := c.Run(ctx); err != nil {
		return fmt.Errorf("while consuming messages: %w", err)
	}

	return nil
}

func (s *BackupConsumer) String() string {
	return "backup-consumer"
}

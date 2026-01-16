package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/internal/consumer"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Tier2WALServer manages the tier 2 WAL server component.
type Tier2WALServer struct {
	Config *config.ServerConfig
}

// Serve starts the tier 2 WAL server and handles incoming requests.
func (s *Tier2WALServer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier2-grpc")
	ctx = log.IntoContext(ctx, contextLogger)

	// Connects to the Klio repository
	tier2WALFS, err := tier2.ConnectWAL(ctx, &s.Config.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2: %w", err)
	}
	tier2RepoConnection, err := repository.Open(repository.Options{
		FS:       tier2WALFS,
		Password: s.Config.Tier2.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to local repository: %w", err)
	}

	// We use the same configuration as Tier1, but changing the listen address
	walServerConfiguration := s.Config.Tier1.Wal
	walServerConfiguration.ListenAddress = s.Config.Tier2.WALListenAddress

	if err := walserver.Start(
		ctx,
		tier2RepoConnection,
		&walServerConfiguration,
		&s.Config.TLS,
	); err != nil {
		return fmt.Errorf("while starting the WAL server: %w", err)
	}

	return nil
}

func (s *Tier2WALServer) String() string {
	return "tier2-grpc"
}

// Tier2KopiaServer manages the tier 2 Kopia server component.
type Tier2KopiaServer struct {
	Config *config.ServerConfig
}

// Serve starts the tier 2 Kopia server and handles backup operations.
func (s *Tier2KopiaServer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier2-kopia")
	ctx = log.IntoContext(ctx, contextLogger)

	// Create a Kopia configuration for tier2
	tier2ConfigFile, err := os.CreateTemp("", "kopia_tier2_config_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	defer func() {
		if err := os.Remove(tier2ConfigFile.Name()); err != nil {
			contextLogger.Warning(
				"Error while removing temporary configuration file",
				"err", err,
				"tier2ConfigFile", tier2ConfigFile.Name(),
			)
		}
	}()

	// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
	if err := kopiaserver.StartTier2(
		ctx,
		&s.Config.Tier2,
		&s.Config.TLS,
	); err != nil {
		return fmt.Errorf("while running kopia server: %w", err)
	}

	return nil
}

func (s *Tier2KopiaServer) String() string {
	return "tier2-kopia"
}

// Tier2BackupConsumer manages the tier 2 backup consumer that processes backup tasks from the queue.
type Tier2BackupConsumer struct {
	Config *config.ServerConfig
}

// Serve starts the tier 2 backup consumer and processes backup tasks from the queue.
//
//nolint:cyclop
func (s *Tier2BackupConsumer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier2-backup-consumer")
	ctx = log.IntoContext(ctx, contextLogger)

	// Create a Kopia configuration for tier1
	tier1ConfigFile, err := os.CreateTemp("", "kopia_tier1_config_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia Tier 1 config: %w", err)
	}

	defer func() {
		if err := os.Remove(tier1ConfigFile.Name()); err != nil {
			contextLogger.Warning(
				"Error while removing temporary configuration file",
				"err", err,
				"tier1ConfigFile", tier1ConfigFile.Name(),
			)
		}
	}()

	if err := kopiaserver.CreateTier1KopiaConfigFile(
		ctx,
		tier1ConfigFile.Name(),
		&s.Config.Tier1,
	); err != nil {
		return err
	}

	// Create a Kopia configuration for tier2
	tier2ConfigFile, err := os.CreateTemp("", "kopia_tier2_config_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia Tier 2 config: %w", err)
	}

	defer func() {
		if err := os.Remove(tier2ConfigFile.Name()); err != nil {
			contextLogger.Warning(
				"Error while removing temporary configuration file",
				"err", err,
				"tier2ConfigFile", tier2ConfigFile.Name(),
			)
		}
	}()

	if err := kopiaserver.CreateTier2KopiaConfigFile(
		ctx,
		tier2ConfigFile.Name(),
		&s.Config.Tier2,
	); err != nil {
		return err
	}

	// Connect to NATS
	natsConnection, err := nats.Connect(
		s.Config.Tier1.Wal.NATSAddress,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("error while connecting to the NATS server: %w", err)
	}
	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		return fmt.Errorf("error while setting up the NATS server: %w", err)
	}

	// Starts the consumer
	c, err := consumer.NewBackup(&consumer.BackupOptions{
		Queue:              queueConnection,
		Tier1KopiaConfig:   tier1ConfigFile.Name(),
		Tier2KopiaConfig:   tier2ConfigFile.Name(),
		CacheDirectory:     s.Config.Tier1.Base.CacheDirectory,
		Tier1EncryptionKey: s.Config.Tier1.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("error while creating backup consumer: %w", err)
	}

	if err := c.Run(ctx); err != nil {
		return fmt.Errorf("while consuming messages: %w", err)
	}

	return nil
}

func (s *Tier2BackupConsumer) String() string {
	return "tier2-backup-consumer"
}

// Tier2WALConsumer manages the tier 2 WAL consumer that processes WAL tasks from the queue.
type Tier2WALConsumer struct {
	Config *config.ServerConfig
}

// Serve starts the tier 2 WAL consumer and processes WAL tasks from the queue.
func (s *Tier2WALConsumer) Serve(ctx context.Context) error {
	contextLogger := log.FromContext(ctx).WithName("tier2-wal-consumer")
	ctx = log.IntoContext(ctx, contextLogger)

	// Connect to the Klio repository in Tier 1
	tier1WALFS := afero.NewBasePathFs(afero.NewOsFs(), s.Config.Tier1.Wal.WALPath)
	tier1RepoConnection, err := repository.Open(repository.Options{
		FS:       tier1WALFS,
		Password: s.Config.Tier1.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to local repository: %w", err)
	}

	// Connect to the Klio repository in Tier 2
	tier2WALFS, err := tier2.ConnectWAL(ctx, &s.Config.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2: %w", err)
	}
	tier2RepoConnection, err := repository.Open(repository.Options{
		FS:       tier2WALFS,
		Password: s.Config.Tier2.EncryptionKey,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to local repository: %w", err)
	}

	// Connect to NATS
	natsConnection, err := nats.Connect(
		s.Config.Tier1.Wal.NATSAddress,
		nats.RetryOnFailedConnect(true),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return fmt.Errorf("error while connecting to the NATS server: %w", err)
	}
	queueConnection, err := queue.New(ctx, natsConnection)
	if err != nil {
		return fmt.Errorf("error while setting up the NATS server: %w", err)
	}

	// Starts the consumer
	c := consumer.NewWAL(&consumer.WALOptions{
		Tier1: tier1RepoConnection,
		Tier2: tier2RepoConnection,
		Queue: queueConnection,
	})

	if err := c.Run(ctx); err != nil {
		return fmt.Errorf("while consuming messages: %w", err)
	}

	return nil
}

func (s *Tier2WALConsumer) String() string {
	return "tier2-wal-consumer"
}

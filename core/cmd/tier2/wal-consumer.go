package tier2

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/consumer"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// walConsumerCmd represents the "wal-consumer" command
//
//nolint:gochecknoglobals
var walConsumerCmd = &cobra.Command{
	Use:   "wal-consumer",
	Short: "Consumes WAL messages from the queue and push them into the tier 2",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if err := configuration.RequireTier1(); err != nil {
			return fmt.Errorf("tier 1 configuration validation error: %w", err)
		}

		if err := configuration.RequireTier2(); err != nil {
			return fmt.Errorf("tier 2 configuration validation error: %w", err)
		}

		if !configuration.Tier2.S3.Enabled {
			// Nothing to be done. There is no tier2.
			return nil
		}

		// Connect to the Klio repository in Tier 1
		tier1WALFS := afero.NewBasePathFs(afero.NewOsFs(), configuration.Tier1.Wal.WALPath)
		tier1RepoConnection, err := repository.Open(repository.Options{
			FS:       tier1WALFS,
			Password: configuration.Tier1.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Connect to the Klio repository in Tier 2
		tier2WALFS, err := tier2.ConnectWAL(cmd.Context(), &configuration.Tier2)
		if err != nil {
			return fmt.Errorf("error while connecting to tier2: %w", err)
		}
		tier2RepoConnection, err := repository.Open(repository.Options{
			FS:       tier2WALFS,
			Password: configuration.Tier2.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		// Connect to NATS
		natsConnection, err := nats.Connect(
			configuration.Tier1.Wal.NATSAddress,
			nats.RetryOnFailedConnect(true),
			nats.ReconnectWait(1*time.Second),
		)
		if err != nil {
			return fmt.Errorf("error while connecting to the NATS server: %w", err)
		}
		queueConnection, err := queue.New(cmd.Context(), natsConnection)
		if err != nil {
			return fmt.Errorf("error while setting up the NATS server: %w", err)
		}

		// Starts the consumer
		c := consumer.NewWAL(&consumer.WALOptions{
			Tier1: tier1RepoConnection,
			Tier2: tier2RepoConnection,
			Queue: queueConnection,
		})

		if err := c.Run(cmd.Context()); err != nil {
			return fmt.Errorf("while consuming messages: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	Tier2Cmd.AddCommand(walConsumerCmd)
}

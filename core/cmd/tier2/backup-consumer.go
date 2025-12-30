package tier2

import (
	"fmt"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/consumer"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// backupConsumerCmd represents the "backup-consumer" command
//
//nolint:gochecknoglobals
var backupConsumerCmd = &cobra.Command{
	Use:   "backup-consumer",
	Short: "Consumes Backup messages from the queue and push them into the tier 2",
	RunE: func(cmd *cobra.Command, _ []string) error {
		contextLogger := log.FromContext(cmd.Context())

		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		if !configuration.Tier2.S3.Enabled {
			// Nothing to be done. There is no tier2.
			return nil
		}

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
			cmd.Context(),
			tier1ConfigFile.Name(),
			&configuration.Tier1,
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
			cmd.Context(),
			tier2ConfigFile.Name(),
			&configuration.Tier2,
		); err != nil {
			return err
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
		c, err := consumer.NewBackup(&consumer.BackupOptions{
			Queue:                   queueConnection,
			Tier1KopiaConfig:        tier1ConfigFile.Name(),
			Tier2KopiaConfig:        tier2ConfigFile.Name(),
			CacheDirectory:          configuration.Tier1.Base.CacheDirectory,
			Tier1EncryptionPassword: configuration.Tier1.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("error while creating backup consumer: %w", err)
		}

		if err := c.Run(cmd.Context()); err != nil {
			return fmt.Errorf("while consuming messages: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	Tier2Cmd.AddCommand(backupConsumerCmd)
}

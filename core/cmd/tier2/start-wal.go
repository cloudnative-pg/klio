package tier2

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startWALCmd represents the start command
//
//nolint:gochecknoglobals
var startWALCmd = &cobra.Command{
	Use:   "start-wal",
	Short: "Starts a Tier 2 Klio WAL server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if err := configuration.RequireTier2(); err != nil {
			return fmt.Errorf("tier 2 configuration validation error: %w", err)
		}

		// Connects to the Klio repository
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

		// We use the same configuration as Tier1, but changing the listen address
		walServerConfiguration := configuration.Tier1.Wal
		walServerConfiguration.ListenAddress = configuration.Tier2.WALListenAddress

		if err := walserver.Start(
			cmd.Context(),
			tier2RepoConnection,
			&walServerConfiguration,
			&configuration.TLS,
		); err != nil {
			return fmt.Errorf("while starting the WAL server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	Tier2Cmd.AddCommand(startWALCmd)
}

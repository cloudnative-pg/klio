package server

import (
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startWALCmd represents the start command
//
//nolint:gochecknoglobals
var startWALCmd = &cobra.Command{
	Use:   "start-wal",
	Short: "Starts a Klio WAL server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		// Connects to the Klio repository
		walFS := afero.NewBasePathFs(afero.NewOsFs(), configuration.Wal.WALPath)
		repoConnection, err := repository.Open(repository.Options{
			FS:       walFS,
			Password: configuration.Wal.EncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to connect to local repository: %w", err)
		}

		if err := walserver.Start(cmd.Context(), repoConnection, &configuration.Wal, &configuration.TLS); err != nil {
			return fmt.Errorf("while starting the WAL server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(startWALCmd)
}

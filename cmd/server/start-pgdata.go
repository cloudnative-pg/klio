package server

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/server/kopiaserver"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// startPgDataCmd represents the start command
//
//nolint:gochecknoglobals
var startPgDataCmd = &cobra.Command{
	Use:   "start-pgdata",
	Short: "Starts a Klio PGDATA server (kopia)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Server == nil {
			return ErrKlioServerSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		log := slog.Default()
		if err := kopiaserver.Start(cmd.Context(), log, configuration.Server.Kopia); err != nil {
			return fmt.Errorf("while running kopia server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(startPgDataCmd)
}

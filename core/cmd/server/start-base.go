package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startBase represents the start command
//
//nolint:gochecknoglobals
var startBase = &cobra.Command{
	Use:   "start-base",
	Short: "Starts a Klio base server",
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

		if err := kopiaserver.StartTier1(cmd.Context(), &configuration.Base); err != nil {
			return fmt.Errorf("while running kopia server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(startBase)
}

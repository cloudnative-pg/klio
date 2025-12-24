package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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

		if err := configuration.RequireTier1(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
		}

		if err := kopiaserver.StartTier1(cmd.Context(), &configuration.Tier1, &configuration.TLS); err != nil {
			return fmt.Errorf("while running kopia server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(startBase)
}

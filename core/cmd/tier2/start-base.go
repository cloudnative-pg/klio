package tier2

import (
	"fmt"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
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
	Short: "Starts a Tier2 Klio base server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		contextLogger := log.FromContext(cmd.Context())

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
		if err := kopiaserver.StartTier2(cmd.Context(), &configuration.Base, &configuration.Tier2); err != nil {
			return fmt.Errorf("while running kopia server: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	Tier2Cmd.AddCommand(startBase)
}

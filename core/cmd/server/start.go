package server

import (
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startCmd represents the start command
//
//nolint:gochecknoglobals
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts a Klio server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig
		contextLogger := log.FromContext(cmd.Context())

		// Generate a unique run-id for this server invocation
		runID := uuid.New()
		runSecret := uuid.New()
		contextLogger.Info("Starting Klio server", "run-id", runID.String())

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		tier1Enabled, err := cmd.Flags().GetBool("tier1")
		if err != nil {
			return fmt.Errorf("failed to read tier1 flag: %w", err)
		}

		tier2Enabled, err := cmd.Flags().GetBool("tier2")
		if err != nil {
			return fmt.Errorf("failed to read tier2 flag: %w", err)
		}

		if !tier1Enabled && !tier2Enabled {
			return errors.New("at least one of --tier1 or --tier2 must be enabled")
		}

		opts := serverOpts{
			tier1:     tier1Enabled,
			tier2:     tier2Enabled,
			cfg:       &configuration,
			runID:     runID.String(),
			runSecret: runSecret.String(),
		}

		// Phase 1: initialize
		if err := initializeRepository(cmd.Context(), opts); err != nil {
			return err
		}

		// Phase 2: start server
		return runServer(cmd.Context(), opts)
	},
}

//nolint:gochecknoinits
func init() {
	startCmd.Flags().Bool("tier1", true, "Enables Tier1 server components")
	startCmd.Flags().Bool("tier2", false, "Enables Tier2 server components")

	ServerCmd.AddCommand(startCmd)
}

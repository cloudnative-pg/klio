// Package cmd is the implementation of the "klio" command
package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/grpcclient"
	"github.com/EnterpriseDB/klio/internal/client/sendwal"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// resetLSNCommand represents the run command
//
//nolint:gochecknoglobals
var resetLSNCommand = &cobra.Command{
	Use:   "reset-lsn",
	Short: "Reset the replication status to the latest flush LSN",
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := slog.Default()

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Source == nil {
			return ErrSourceSectionIsRequired
		}

		if configuration.Client == nil {
			return ErrClientSectionIsRequired
		}

		if configuration.Client.Klio == nil {
			return ErrKlioClientSectionIsRequired
		}

		logger.Debug("Current configuration", "configuration", configuration)
		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		client, err := grpcclient.Connect(
			logger,
			configuration.Client.Klio,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		return sendwal.
			New(&configuration, logger, client).
			ResetReplicationStatus(cmd.Context())
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(resetLSNCommand)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// Package cmd is the implementation of the "run" command
package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
	"github.com/EnterpriseDB/klio/internal/client/klioclient/grpcclient"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// getWalCmd represents the run command
//
//nolint:gochecknoglobals
var getWalCmd = &cobra.Command{
	Use:   "get-wal [wal-name] [target-file]",
	Short: "Get a WAL from the target Klio server",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.Default()

		walName := args[0]
		targetFileName := args[1]

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

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

		var client common.WALClientStreamer
		var err error

		if client, err = grpcclient.Connect(
			logger,
			configuration.Client.Klio,
		); err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		output, err := os.OpenFile(targetFileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
		if err != nil {
			return fmt.Errorf("cannot open file %s: %w", targetFileName, err)
		}
		defer func() {
			if closeErr := output.Close(); closeErr != nil {
				logger.Error("While closing WAL file", "err", closeErr)
			}
		}()

		if err := client.GetWALStreaming(cmd.Context(), walName, output); err != nil {
			if errors.Is(err, common.ErrMissingWALFile) {
				logger.Error("missing WAL file, exiting with error code 1", "wal_name", walName)
				os.Exit(1)
			}
			return fmt.Errorf("while downloading WAL: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(getWalCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

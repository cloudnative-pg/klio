package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// getWalCmd represents the run command
//
//nolint:gochecknoglobals
var getWalCmd = &cobra.Command{
	Use:   "get-wal [wal-name] [target-file]",
	Short: "Get a WAL from the target Klio server",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextLogger := log.FromContext(cmd.Context())

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

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return cli.ErrKlioClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		var downloadPartial bool

		downloadPartial, _ = cmd.Flags().GetBool("partial")
		tier2, _ := cmd.Flags().GetBool("tier2")

		address := configuration.Client.Wal.Address
		if tier2 {
			address = configuration.Client.Wal.Tier2Address
		}
		if address == "" {
			os.Exit(4)
		}

		client, err := grpcclient.Connect(&configuration.Client.Wal, address)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		output, err := os.OpenFile(targetFileName, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
		if err != nil {
			return fmt.Errorf("cannot open file %s: %w", targetFileName, err)
		}
		defer func() {
			if closeErr := output.Close(); closeErr != nil {
				contextLogger.Error(closeErr, "While closing WAL file")
			}
		}()

		// Try to download the requested WAL file. If we did it, everything
		// is fine.
		err = client.GetWALStreaming(cmd.Context(), walName, output)
		switch {
		case errors.Is(err, klioclient.ErrMissingWALFile):
			if path.Ext(walName) != "" || !downloadPartial {
				contextLogger.Debug("Missing WAL file, exiting with error code 4", "wal_name", walName)
				os.Exit(4)
			}

		case err != nil:
			return fmt.Errorf("unknown error: %w", err)

		default:
			return nil
		}

		// Let's try downloading the partial file
		walName += ".partial"
		err = client.GetWALStreaming(cmd.Context(), walName, output)

		var incompleteError klioclient.IncompleteTransmissionError
		switch {
		case errors.As(err, &incompleteError):
			contextLogger.Error(err, "Incomplete partial WAL file, exiting with error code 1", "wal_name", walName)
			return err

		case errors.Is(err, klioclient.ErrMissingWALFile):
			contextLogger.Debug("Missing partial WAL file, exiting with error code 4", "wal_name", walName)
			os.Exit(4)

		case err != nil:
			return fmt.Errorf("unknown error: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(getWalCmd)

	getWalCmd.Flags().Bool(
		"partial",
		false,
		"Use a partial WAL file if a the completed WAL file is not present. Defaults to false",
	)

	getWalCmd.Flags().Bool(
		"tier2",
		false,
		"Look in tier2 instead of in tier1, if tier2 is available.",
	)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

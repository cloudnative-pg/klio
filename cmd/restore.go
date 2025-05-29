package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
	"github.com/EnterpriseDB/klio/internal/client/klioclient/kopia"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// restoreCmd represents the restore command
//
//nolint:gochecknoglobals
var restoreCmd = &cobra.Command{
	Use:   "restore [backup-name] [destination]",
	Short: "Restore a PostgreSQL cluster from a Klio server",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.Default()

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
		if configuration.Client.Kopia == nil {
			return ErrKopiaClientSectionIsRequired
		}

		logger.Debug("Current configuration", "configuration", configuration)
		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		backupName := args[0]
		destinationPath := args[1]
		tablespaces := make(map[string]string)

		optionSlice, err := cmd.Flags().GetStringSlice("tablespaces")
		if err != nil {
			return fmt.Errorf("could not parse tablespaces: %w", err)
		}

		for _, option := range optionSlice {
			splittedOption := strings.Split(option, ":")
			if len(splittedOption) != 2 || len(splittedOption[0]) == 0 || len(splittedOption) == 0 {
				return newInvalidTablespaceRemapOptionError(option)
			}

			tablespaces[splittedOption[0]] = splittedOption[1]
		}

		client, err := kopia.Connect(
			cmd.Context(),
			logger,
			configuration.Client.Kopia,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		restoreOptions := common.RestoreOptions{
			Name:                 backupName,
			PgDataDirectory:      destinationPath,
			TablespacesDirectory: tablespaces,
			Progress:             common.NewDownloadProgressLogger(logger),
		}

		executor, err := client.CreateRestoreExecutor(cmd.Context(), restoreOptions)
		if err != nil {
			return fmt.Errorf("while creating restore executor: %w", err)
		}

		return executor.Restore(cmd.Context(), destinationPath)
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(restoreCmd)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	restoreCmd.Flags().StringSlice(
		"tablespaces",
		nil,
		"A comma-separated list of tablespace_name:tablespace_path to customize "+
			"the restore location of a tablespace.")
}

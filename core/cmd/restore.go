package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/notifier"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// restoreCmd represents the restore command
//
//nolint:gochecknoglobals
var restoreCmd = &cobra.Command{
	Use:   "restore [destination]",
	Short: "Restore a PostgreSQL cluster from a Klio server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		contextLogger := log.FromContext(cmd.Context())

		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the default values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}
		if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
			return cli.ErrKopiaClientSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		destinationPath := args[0]
		backupName, _ := cmd.Flags().GetString("backup-id")
		tablespaces := make(map[string]string)

		targetTimeString, _ := cmd.Flags().GetString("target-time")
		var targetTime time.Time

		if targetTimeString != "" {
			var err error

			targetTime, err = types.ParseTargetTime(nil, targetTimeString)
			if err != nil {
				return fmt.Errorf("unable to parse target time %q: %w", targetTimeString, err)
			}
		}

		optionSlice, err := cmd.Flags().GetStringSlice("tablespaces")
		if err != nil {
			return fmt.Errorf("could not parse tablespaces: %w", err)
		}

		for _, option := range optionSlice {
			splitOption := strings.Split(option, ":")
			if len(splitOption) != 2 || len(splitOption[0]) == 0 || len(splitOption) == 0 {
				return newInvalidTablespaceRemapOptionError(option)
			}

			tablespaces[splitOption[0]] = splitOption[1]
		}

		client, err := kopia.Connect(cmd.Context(), &configuration.Client.Base)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		restorer := client.CreateRestorer(
			notifier.NewDownloadLogNotifier(contextLogger),
			kopia.Target{
				Hostname: client.GetHostname(),
				Username: client.GetUsername(),
			},
		)

		if backupName == "" {
			contextLogger := contextLogger.WithValues("targetTime", targetTime)
			backupList, err := restorer.ListBackups(cmd.Context())
			if err != nil {
				return fmt.Errorf("while downloading the backup catalog: %w", err)
			}

			contextLogger.Info(
				"Downloaded backup catalog",
				"backupList", backupList,
				"clusterName", client.GetHostname(),
				"userName", client.GetUsername(),
			)

			var targetBackup *common.BackupMetadata
			if !targetTime.IsZero() {
				targetBackup = backupList.FindClosestBackup(targetTime)
			} else {
				targetBackup = backupList.GetLatestBackup()
			}

			if targetBackup == nil {
				contextLogger.Info("Unable to find a suitable backup to restore, bailing out")
			} else {
				contextLogger.Info("Found target backup", "targetBackup", targetBackup)
				backupName = targetBackup.Name
			}
		}

		executor := common.NewRestoreExecutor(
			restorer,
			common.RestoreConfiguration{
				Name:                 backupName,
				PgDataDirectory:      destinationPath,
				TablespacesDirectory: tablespaces,
			},
		)

		return executor.Restore(cmd.Context(), destinationPath)
	},
}

type invalidTablespaceRemapOptionError struct {
	Option string
}

func newInvalidTablespaceRemapOptionError(option string) *invalidTablespaceRemapOptionError {
	return &invalidTablespaceRemapOptionError{
		Option: option,
	}
}

func (e *invalidTablespaceRemapOptionError) Error() string {
	return "invalid tablespace remap option " + e.Option
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

	restoreCmd.Flags().String(
		"backup-id",
		"",
		"The ID of the backup to be restored. Defaults to the latest backup found "+
			"for the cluster",
	)

	restoreCmd.Flags().String(
		"target-time",
		"",
		"If specified, Klio will recover from the most recent backup that was taken "+
			"before the time specified. This allows to configure PostgreSQL to do a "+
			"point-in-time recovery with the specified target time.",
	)
}

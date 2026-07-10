package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// errNoSuitableBackup is returned when no backup matches the requested recovery
// target: the latest backup, or the closest one before a target time.
var errNoSuitableBackup = errors.New("no suitable backup found to restore for the requested recovery target")

// errNoTierForRecovery is returned when neither a tier1 nor a tier2 base URL
// is configured for recovery.
var errNoTierForRecovery = errors.New("no repository is configured for recovery")

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

		if err := configuration.Validate(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
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

		if dropTier2BaseURLWhenRecoveryDisabled(&configuration) {
			contextLogger.Info(
				"tier2 is configured for backup only; recovery from tier2 is not enabled, " +
					"so base backups stored only on tier2 will not be used for this recovery. " +
					"Enable tier2 recovery to restore from tier2.")
		}

		if configuration.Client.Base.URL == "" && configuration.Client.Base.Tier2URL == "" {
			return errNoTierForRecovery
		}

		client, err := kopia.MultiConnect(cmd.Context(), &configuration.Client)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}
		defer client.Close(cmd.Context())

		if backupName == "" {
			contextLogger := contextLogger.WithValues("targetTime", targetTime)
			backupList, err := client.ListBackups(cmd.Context(), client.GetHostname())
			if err != nil {
				return fmt.Errorf("while downloading the backup catalog: %w", err)
			}

			contextLogger.Info(
				"Downloaded backup catalog",
				"backupList", backupList,
				"clusterName", client.GetHostname(),
				"userName", client.GetUsername(),
			)

			var targetBackup *klioclient.BackupMetadata
			if !targetTime.IsZero() {
				targetBackup = backupList.FindClosestBackup(targetTime)
			} else {
				targetBackup = backupList.GetLatestBackup()
			}

			if targetBackup == nil {
				return errNoSuitableBackup
			}

			contextLogger.Info("Found target backup", "targetBackup", targetBackup)
			backupName = targetBackup.Name
		}

		executor := klioclient.NewRestoreExecutor(
			client,
			klioclient.RestoreConfiguration{
				Name:                 backupName,
				PgDataDirectory:      destinationPath,
				TablespacesDirectory: tablespaces,
			},
		)

		return executor.Restore(cmd.Context(), client.GetHostname(), destinationPath)
	},
}

// dropTier2BaseURLWhenRecoveryDisabled clears Client.Base.Tier2URL so restore
// never reads base backups from a backup-only tier2. The operator populates
// Tier2URL whenever tier2 is enabled for backup OR recovery, so URL presence
// alone does not imply recovery intent; the WAL restore path applies the same
// gate via availableTiers. Reports whether the URL was dropped.
func dropTier2BaseURLWhenRecoveryDisabled(configuration *config.Data) bool {
	if configuration.Tier2RecoveryEnabled || configuration.Client.Base.Tier2URL == "" {
		return false
	}

	configuration.Client.Base.Tier2URL = ""

	return true
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

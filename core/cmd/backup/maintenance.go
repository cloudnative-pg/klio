package backup

import (
	"fmt"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// maintenanceCmd represents the klio backup maintenance command
//
//nolint:gochecknoglobals
var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Gets the metadata of all backups",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data
		contextLogger := log.FromContext(cmd.Context())

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
		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return cli.ErrKlioClientSectionIsRequired
		}
		if configuration.Source == (config.SourceConfig{}) {
			return cli.ErrSourceSectionIsRequired
		}

		if err := configuration.Validate(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
		}

		kopiaClient, err := kopia.MultiConnect(
			cmd.Context(),
			&configuration.Client.Base,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL)
		}

		target := klioclient.Target{
			Hostname: kopiaClient.GetHostname(),
			Username: kopiaClient.GetUsername(),
		}

		if err := kopiaClient.ApplyRetentionPolicy(cmd.Context(), target); err != nil {
			return fmt.Errorf("while applying the retention policy: %w", err)
		}

		klioClient, err := grpcclient.Connect(&configuration.Client.Wal, configuration.Client.Wal.Address)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		// Step 1: list in-use backups
		backups, err := kopiaClient.ListBackups(cmd.Context(), kopiaClient.GetHostname())
		if err != nil {
			return fmt.Errorf("while extracting backups: %w %q", err, configuration.Client.Base.URL)
		}

		// No backup has been found, we can't do anything
		if len(backups) == 0 {
			contextLogger.Info("No backups found")
			return nil
		}

		// Step 2: find the oldest WAL file
		oldestWAL := ""
		for _, backup := range backups {
			if oldestWAL == "" || strings.Compare(backup.StartWAL, oldestWAL) == -1 {
				oldestWAL = backup.StartWAL
			}
		}

		if oldestWAL == "" {
			return nil
		}

		contextLogger.Info("Oldest in-use WAL file", "oldestWAL", oldestWAL)

		// Step 3: drop all the unneeded WAL files
		if err := klioClient.SetFirstRequiredWAL(cmd.Context(), oldestWAL); err != nil {
			contextLogger.Error(err, "while executing server-side retention policy", "oldestWAL", oldestWAL)
			return err
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	BackupCmd.AddCommand(maintenanceCmd)
}

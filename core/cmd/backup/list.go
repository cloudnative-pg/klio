package backup

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// listCmd represents the klio backup get-metadata command
//
//nolint:gochecknoglobals
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Gets the metadata of all backups",
	RunE: func(cmd *cobra.Command, _ []string) error {
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
		if configuration.Source == (config.SourceConfig{}) {
			return cli.ErrSourceSectionIsRequired
		}

		if err := configuration.Validate(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
		}

		client, err := kopia.MultiConnect(
			cmd.Context(),
			&configuration.Client.Base,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL)
		}

		backups, err := client.ListBackups(cmd.Context(), client.GetHostname())
		if err != nil {
			return fmt.Errorf("while extracting backups: %w %q", err, configuration.Client.Base.URL)
		}

		jsonData, err := json.Marshal(backups)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata to JSON: %w", err)
		}

		_, err = os.Stdout.Write(jsonData)
		if err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
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

	BackupCmd.AddCommand(listCmd)
}

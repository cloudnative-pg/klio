package backup

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// runCmd represents the run command
//
//nolint:gochecknoglobals
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Backup the PostgreSQL cluster to the opened Klio server",
	RunE: func(cmd *cobra.Command, _ []string) error {
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
		if configuration.Client.Base == (config.BaseRepositoryClientConfig{}) {
			return cli.ErrKopiaClientSectionIsRequired
		}
		if configuration.Source == (config.SourceConfig{}) {
			return cli.ErrSourceSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		client, err := kopia.Connect(
			cmd.Context(),
			&configuration.Client.Base,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL)
		}

		conn, err := pgx.Connect(cmd.Context(), configuration.Source.StandardDSN)
		if err != nil {
			return fmt.Errorf("while connecting to PostgreSQL: %w", err)
		}
		defer func() {
			_ = conn.Close(cmd.Context())
		}()

		uploader := client.NewUploaderFor(
			kopia.Target{
				Hostname: client.GetHostname(),
				Username: client.GetUsername(),
			},
		)
		backupExecutor := common.NewBackupExecutor(conn, uploader, configuration.Client.Base.Hostname)

		var opts common.BackupOptions

		backupName, _ := cmd.Flags().GetString("name")
		opts.Name = backupName

		if err := backupExecutor.Start(cmd.Context(), opts); err != nil {
			return fmt.Errorf("while starting the backup: %w", err)
		}

		if err := backupExecutor.Upload(cmd.Context()); err != nil {
			return fmt.Errorf("while uploading data: %w", err)
		}

		metadata, err := backupExecutor.Close(cmd.Context())
		if err != nil {
			return fmt.Errorf("while closing the backup: %w", err)
		}

		// Marshal metadata to JSON
		jsonData, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata to JSON: %w", err)
		}

		fmt.Println() //nolint:forbidigo

		fmt.Print(string(jsonData)) //nolint:forbidigo

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	// Here you will define your flags and configuration settings.
	runCmd.Flags().StringP("name", "n", "", "The backup name")

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	BackupCmd.AddCommand(runCmd)
}

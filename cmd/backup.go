// Package cmd is the implementation of the "run" command
package cmd

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
	"github.com/EnterpriseDB/klio/pkg/klioclient/kopia"
)

// ErrKopiaClientSectionIsRequired is raised when the kopia client configuration is missing.
var ErrKopiaClientSectionIsRequired = errors.New("'client.kopia' configuration section is required")

// backupCmd represents the backup command
//
//nolint:gochecknoglobals
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup the PostgreSQL cluster to the opened Klio server",
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

		client, err := kopia.Connect(
			cmd.Context(),
			logger,
			configuration.Client.Kopia,
		)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		dsn, _ := cmd.Flags().GetString("dsn")

		conn, err := pgx.Connect(cmd.Context(), dsn)
		if err != nil {
			return fmt.Errorf("while connecting to PostgreSQL: %w", err)
		}
		defer func() {
			_ = conn.Close(cmd.Context())
		}()

		backupExecutor, err := client.CreateBackup(cmd.Context(), common.BackupOptions{
			Connection: conn,
			Progress:   common.NewLoggerProgress(logger),
		})
		if err != nil {
			return fmt.Errorf("while creating a backup executor: %w", err)
		}

		if err := backupExecutor.Start(cmd.Context()); err != nil {
			return fmt.Errorf("while starting the backup: %w", err)
		}

		if err := backupExecutor.Upload(cmd.Context()); err != nil {
			return fmt.Errorf("while uploading data: %w", err)
		}

		if err := backupExecutor.Close(cmd.Context()); err != nil {
			return fmt.Errorf("while closing the backup: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(backupCmd)

	backupCmd.Flags().StringP("dsn", "d", "", "The DSN to create a superuser PostgreSQL connection")
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

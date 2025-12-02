package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// runCmd represents the `backup run` command
//
//nolint:gochecknoglobals
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Backup the PostgreSQL cluster to the opened Klio server",
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

		waitWALs, _ := cmd.Flags().GetBool("wait-for-wals")

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		kopiaClient, err := kopia.Connect(
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

		uploader := kopiaClient.NewUploaderFor(
			kopia.Target{
				Hostname: kopiaClient.GetHostname(),
				Username: kopiaClient.GetUsername(),
			},
		)
		backupExecutor := common.NewBackupExecutor(conn, uploader, kopiaClient.GetHostname())

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

		grpcClient, err := grpcclient.Connect(&configuration.Client.Wal)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		for {
			result, err := grpcClient.CloseBackup(cmd.Context(), &grpc.CloseBackupRequest{
				ClusterName:      kopiaClient.GetHostname(),
				KopiaSourceNames: metadata.Sources,
				BackupName:       metadata.Name,
				Timeline:         int32(metadata.Timeline),
				StartWal:         metadata.StartWAL,
				EndWal:           metadata.EndWAL,
				SegmentSize:      metadata.SegmentSize,
			})
			if err != nil {
				return fmt.Errorf("while closing the backup: %w", err)
			}

			if waitWALs && len(result.GetMissingWalFiles()) > 0 {
				contextLogger.Info(
					"Detected missing WAL files, waiting for 5 seconds",
					"missingWALFiles", result.GetMissingWalFiles(),
				)
				time.Sleep(5 * time.Second)

				continue
			}

			if result.GetTier2Schedule() {
				contextLogger.Info("Backup completed, triggered synchronization to tier2 if available")
			}

			break
		}

		// Marshal metadata to JSON
		if err := json.NewEncoder(os.Stdout).Encode(metadata); err != nil {
			return fmt.Errorf("failed to marshal metadata to JSON: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	// Here you will define your flags and configuration settings.
	runCmd.Flags().StringP("name", "n", "", "The backup name")

	runCmd.Flags().Bool(
		"wait-for-wals",
		true,
		"When enabled, wait until all the required WAL files have "+
			"been archived in tier1 before declaring the backup completed.")

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	BackupCmd.AddCommand(runCmd)
}

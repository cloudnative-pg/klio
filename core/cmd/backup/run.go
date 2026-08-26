/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	kopiaWrapper "github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// runCmd represents the `backup run` command
//
//nolint:gochecknoglobals
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Backup the PostgreSQL cluster to the opened Klio server",
	RunE:  cli.RunEWithExitCode(runBackup),
}

//nolint:cyclop
func runBackup(cmd *cobra.Command, _ []string) error {
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
	tier2, _ := cmd.Flags().GetBool("enable-tier2-backup")

	if err := configuration.Validate(); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}

	kopiaClient, err := kopia.MultiConnect(
		cmd.Context(),
		&configuration.Client,
	)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL),
			backupfailure.RepositoryError.ExitCode)
	}
	defer kopiaClient.Close(cmd.Context())

	// Overrides the repository's global policy for this cluster's source.
	if err := setTier1CompressionPolicy(cmd.Context(), kopiaClient, &configuration); err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while setting the tier1 compression policy: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	conn, err := pgx.Connect(cmd.Context(), configuration.Source.StandardDSN)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while connecting to PostgreSQL: %w", err),
			backupfailure.SourceError.ExitCode)
	}
	defer func() {
		_ = conn.Close(cmd.Context())
	}()

	backupExecutor := klioclient.NewBackupExecutor(conn, kopiaClient, kopiaClient.GetHostname())

	var opts klioclient.BackupOptions

	backupName, _ := cmd.Flags().GetString("name")
	opts.Name = backupName

	if err := backupExecutor.Start(cmd.Context(), opts); err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while starting the backup: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	if err := backupExecutor.Upload(cmd.Context(), tier2); err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while uploading data: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	metadata, err := backupExecutor.Close(cmd.Context(), tier2)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while closing the backup: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	grpcClient, err := grpcclient.Connect(&configuration.Client, configuration.Client.Wal.Address)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while connecting to the Klio server: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	for {
		//nolint:gosec // postgres timeline is uint32 in practice, fits int32
		timeline := int32(metadata.Timeline)

		result, err := grpcClient.CloseBackup(cmd.Context(), &grpc.CloseBackupRequest{
			ClusterName:            kopiaClient.GetHostname(),
			BackupName:             metadata.Name,
			Timeline:               timeline,
			StartWal:               metadata.StartWAL,
			EndWal:                 metadata.EndWAL,
			SegmentSize:            metadata.SegmentSize,
			SendToTier2:            tier2,
			Tier2RetentionPolicy:   marshalTier2RetentionPolicy(cmd.Context(), &configuration),
			Tier2CompressionPolicy: marshalTier2CompressionPolicy(cmd.Context(), &configuration),
		})
		if err != nil {
			return cli.NewCodedError(
				fmt.Errorf("while closing the backup: %w", err),
				backupfailure.RepositoryError.ExitCode)
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
}

// setTier1CompressionPolicy applies the per-cluster tier1 compression policy,
// if configured, to the cluster's source in the tier1 repository.
func setTier1CompressionPolicy(
	ctx context.Context,
	client *kopia.MultiConnection,
	configuration *config.Data,
) error {
	policy := toKopiaCompressionPolicy(configuration.Tier1CompressionPolicy)
	if policy.IsZero() {
		return nil
	}

	target := kopiaWrapper.Target{
		Username: client.GetUsername(),
		Hostname: client.GetHostname(),
	}

	return client.SetCompressionPolicy(ctx, target, policy)
}

// toKopiaCompressionPolicy converts a config compression policy into the Kopia
// wrapper representation. A nil input yields the zero policy.
func toKopiaCompressionPolicy(p *config.CompressionPolicy) kopiaWrapper.CompressionPolicy {
	if p == nil {
		return kopiaWrapper.CompressionPolicy{}
	}

	return kopiaWrapper.CompressionPolicy{
		Algorithm: p.Algorithm,
		MinSize:   p.MinSize,
		MaxSize:   p.MaxSize,
	}
}

// marshalTier2RetentionPolicy serializes the tier2 retention policy to the
// JSON representation expected by the WAL server. It returns an empty string
// when no policy is configured or serialization fails.
func marshalTier2RetentionPolicy(ctx context.Context, configuration *config.Data) string {
	if configuration.Tier2RetentionPolicy == nil {
		return ""
	}

	policy := kopiaWrapper.RetentionPolicy{
		KeepLatest:  configuration.Tier2RetentionPolicy.KeepLatest,
		KeepHourly:  configuration.Tier2RetentionPolicy.KeepHourly,
		KeepDaily:   configuration.Tier2RetentionPolicy.KeepDaily,
		KeepWeekly:  configuration.Tier2RetentionPolicy.KeepWeekly,
		KeepMonthly: configuration.Tier2RetentionPolicy.KeepMonthly,
		KeepAnnual:  configuration.Tier2RetentionPolicy.KeepAnnual,
	}

	content, err := json.Marshal(policy)
	if err != nil {
		log.FromContext(ctx).Error(err, "Error while serializing the tier2 retention policy, skipping")

		return ""
	}

	return string(content)
}

// marshalTier2CompressionPolicy serializes the tier2 compression policy to the
// JSON representation expected by the WAL server. It returns an empty string
// when no policy is configured or serialization fails.
func marshalTier2CompressionPolicy(ctx context.Context, configuration *config.Data) string {
	policy := toKopiaCompressionPolicy(configuration.Tier2CompressionPolicy)
	if policy.IsZero() {
		return ""
	}

	content, err := json.Marshal(policy)
	if err != nil {
		log.FromContext(ctx).Error(err, "Error while serializing the tier2 compression policy, skipping")

		return ""
	}

	return string(content)
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

	runCmd.Flags().Bool(
		"enable-tier2-backup",
		false,
		"When enabled, require the backup to be sent to tier2")

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	BackupCmd.AddCommand(runCmd)
}

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

package retention

import (
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// applyCmd represents the `retention apply` command.
//
//nolint:gochecknoglobals
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply the configured retention policy immediately",
	Long: "Apply the retention policy from the configuration to the target cluster " +
		"without waiting for the next backup, to free space on demand.",
	RunE: cli.RunEWithExitCode(runApply),
}

func runApply(cmd *cobra.Command, _ []string) error {
	contextLogger := log.FromContext(cmd.Context())

	var configuration config.Data

	// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
	// when using environment variables
	if err := viper.Unmarshal(&configuration); err != nil {
		return fmt.Errorf("could not unmarshal configuration: %w", err)
	}

	// Sets the default values, to be overridden by the user configuration.
	configuration.SetDefaults()

	if configuration.Client == (config.ClientConfig{}) {
		return cli.ErrClientSectionIsRequired
	}
	if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
		return cli.ErrKlioClientSectionIsRequired
	}

	if err := configuration.Validate(); err != nil {
		return fmt.Errorf("configuration validation error: %w", err)
	}

	tier1RetentionPolicy, err := configuration.Tier1RetentionPolicy.MarshalWire()
	if err != nil {
		return fmt.Errorf("while serializing the tier1 retention policy: %w", err)
	}

	tier2RetentionPolicy, err := configuration.Tier2RetentionPolicy.MarshalWire()
	if err != nil {
		return fmt.Errorf("while serializing the tier2 retention policy: %w", err)
	}

	grpcClient, err := grpcclient.Connect(&configuration.Client, configuration.Client.Wal.Address)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while connecting to the Klio server: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	result, err := grpcClient.ApplyRetention(cmd.Context(), &grpc.ApplyRetentionRequest{
		ClusterName:          configuration.Client.ClusterName,
		Tier1RetentionPolicy: tier1RetentionPolicy,
		Tier2RetentionPolicy: tier2RetentionPolicy,
	})
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while applying retention: %w", err),
			backupfailure.RepositoryError.ExitCode)
	}

	if result.GetScheduled() {
		contextLogger.Info("Retention apply scheduled", "cluster", configuration.Client.ClusterName)
	}

	return nil
}

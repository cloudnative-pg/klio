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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// setCmd represents the retention get command
//
//nolint:gochecknoglobals
var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Sets the currently applied retention policy",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Source == (config.SourceConfig{}) {
			return cli.ErrSourceSectionIsRequired
		}

		if configuration.Client == (config.ClientConfig{}) {
			return cli.ErrClientSectionIsRequired
		}

		if configuration.Client.Wal == (config.WalRepositoryClientConfig{}) {
			return cli.ErrKlioClientSectionIsRequired
		}

		if err := configuration.Validate(); err != nil {
			return fmt.Errorf("configuration validation error: %w", err)
		}

		client, err := grpcclient.Connect(&configuration.Client, configuration.Client.Wal.Address)
		if err != nil {
			return fmt.Errorf("while connecting to the Klio server: %w", err)
		}

		policyRequest := &grpc.SetRetentionPolicyRequest{
			ClusterName:     configuration.Client.ClusterName,
			RetentionPolicy: toGRPCRetentionPolicy(configuration),
		}
		_, err = client.SetRetentionPolicy(cmd.Context(), policyRequest)
		if err != nil {
			return fmt.Errorf("while setting the current retention policy: %w", err)
		}

		return nil
	},
}

// toGRPCRetentionPolicy converts a config.RetentionPolicy into its gRPC
// wire representation. A nil policy converts to nil.
func toGRPCRetentionPolicy(configuration config.Data) *grpc.RetentionPolicy {
	result := &grpc.RetentionPolicy{}
	tier1RetentionPolicy := &grpc.TierRetentionPolicy{}
	tier2RetentionPolicy := &grpc.TierRetentionPolicy{}
	if configuration.Tier1RetentionPolicy != nil {
		tier1RetentionPolicy.Latest = *configuration.Tier1RetentionPolicy.Latest
		result.Tier1Policy = tier1RetentionPolicy
	}

	if configuration.Tier2RetentionPolicy != nil {
		tier2RetentionPolicy.Latest = *configuration.Tier2RetentionPolicy.Latest
		result.Tier2Policy = tier2RetentionPolicy
	}

	return result
}

//nolint:gochecknoinits
func init() {
	RetentionCmd.AddCommand(setCmd)
}

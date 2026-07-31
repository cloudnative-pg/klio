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

package cmd

import (
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/client/sendwal"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// resetLSNCommand represents the run command
//
//nolint:gochecknoglobals
var resetLSNCommand = &cobra.Command{
	Use:   "reset-lsn",
	Short: "Reset the replication status to the latest flush LSN",
	RunE: func(cmd *cobra.Command, _ []string) error {
		contextLogger := log.FromContext(cmd.Context())

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

		return sendwal.
			New(&configuration, contextLogger, client, false).
			ResetReplicationStatus(cmd.Context())
	},
}

//nolint:gochecknoinits
func init() {
	rootCmd.AddCommand(resetLSNCommand)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

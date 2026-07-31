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
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
	"github.com/cloudnative-pg/klio/core/internal/cli"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// getMetadataCmd represents the klio backup get-metadata command
//
//nolint:gochecknoglobals
var getMetadataCmd = &cobra.Command{
	Use:   "get-metadata [backupName]",
	Short: "Gets the metadata of the backup with the provided name",
	Args:  cobra.ExactArgs(1),
	RunE:  cli.RunEWithExitCode(runGetMetadata),
}

func runGetMetadata(cmd *cobra.Command, args []string) error {
	var configuration config.Data
	backupName := args[0]

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
		&configuration.Client,
	)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while connecting to the Klio server: %w %q", err, configuration.Client.Base.URL),
			backupfailure.RepositoryError.ExitCode)
	}
	defer client.Close(cmd.Context())

	metadata, err := client.GetMetadata(cmd.Context(), client.GetHostname(), backupName)
	if err != nil {
		return cli.NewCodedError(
			fmt.Errorf("while getting metadata for backup %q: %w", backupName, err),
			backupfailure.RepositoryError.ExitCode)
	}

	// Marshal metadata to JSON
	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata to JSON: %w", err)
	}

	if _, err := os.Stdout.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write JSON output: %w", err)
	}

	return nil
}

//nolint:gochecknoinits
func init() {
	// Here you will define your flags and configuration settings.
	getMetadataCmd.Flags().StringP("name", "n", "", "The backup name")

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	BackupCmd.AddCommand(getMetadataCmd)
}

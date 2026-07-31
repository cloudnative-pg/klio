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

package server

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// startCmd represents the start command
//
//nolint:gochecknoglobals
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts a Klio server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		shutdownOtel := opentelemetry.Init(cmd.Context())
		defer shutdownOtel()

		var configuration config.ServerConfig
		contextLogger := log.FromContext(cmd.Context())

		// Generate a unique run-id for this server invocation
		runID := uuid.New()
		runSecret := uuid.New()
		contextLogger.Info("Starting Klio server", "run-id", runID.String())

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		tier1Enabled, err := cmd.Flags().GetBool("tier1")
		if err != nil {
			return fmt.Errorf("failed to read tier1 flag: %w", err)
		}

		tier2Enabled, err := cmd.Flags().GetBool("tier2")
		if err != nil {
			return fmt.Errorf("failed to read tier2 flag: %w", err)
		}

		adminSocketPath, err := cmd.Flags().GetString("socket-path")
		if err != nil {
			return fmt.Errorf("failed to read socketPath flag: %w", err)
		}

		if !tier1Enabled && !tier2Enabled {
			return errors.New("at least one of --tier1 or --tier2 must be enabled")
		}

		if tier1Enabled {
			if err := configuration.Tier1.LoadEncryptionKey(); err != nil {
				return fmt.Errorf("could not load tier1 encryption key: %w", err)
			}
		}

		if tier2Enabled {
			if err := configuration.Tier2.LoadEncryptionKey(); err != nil {
				return fmt.Errorf("could not load tier2 encryption key: %w", err)
			}
		}

		opts := serverOpts{
			tier1:           tier1Enabled,
			tier2:           tier2Enabled,
			cfg:             &configuration,
			runID:           runID.String(),
			runSecret:       runSecret.String(),
			adminSocketPath: adminSocketPath,
		}

		// Phase 1: initialize
		if err := initializeRepository(cmd.Context(), opts); err != nil {
			return err
		}

		// Phase 2: start server
		return runServer(cmd.Context(), opts)
	},
}

//nolint:gochecknoinits
func init() {
	socketPath := path.Join(os.TempDir(), ".klio-admin")
	startCmd.Flags().Bool("tier1", true, "Enables Tier1 server components")
	startCmd.Flags().Bool("tier2", false, "Enables Tier2 server components")
	startCmd.Flags().String("socket-path", socketPath, "Unix socket used by the administration server")

	ServerCmd.AddCommand(startCmd)
}

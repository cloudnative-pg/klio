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

package admin

import (
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// refreshCmd represents the refresh command
//
//nolint:gochecknoglobals
var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh the Kopia cache and policies",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		socketPath, err := cmd.Flags().GetString("socket-path")
		if err != nil {
			return fmt.Errorf("while getting the socketPath flag: %w", err)
		}

		conn, err := connectToAdminServer(socketPath)
		if err != nil {
			return err
		}
		defer func() {
			_ = conn.Close()
		}()

		adminClient := klioGRPC.NewAdminClient(conn)
		if _, err := adminClient.Refresh(cmd.Context(), &klioGRPC.RefreshRequest{}); err != nil {
			return fmt.Errorf("while calling refresh entrypoint: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	AdminCmd.AddCommand(refreshCmd)

	socketPath := path.Join(os.TempDir(), ".klio-admin")
	refreshCmd.Flags().String("socket-path", socketPath, "Unix socket used by the administration server")
}

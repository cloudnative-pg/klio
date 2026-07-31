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

// listBackupsCmd represents the list-backups command
//
//nolint:gochecknoglobals
var listBackupsCmd = &cobra.Command{
	Use:   "list-backups",
	Short: "List the backups available in the Klio server",
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
		backups, err := adminClient.ListBackups(cmd.Context(), &klioGRPC.ListBackupsRequest{})
		if err != nil {
			return fmt.Errorf("while calling list-backups entrypoint: %w", err)
		}

		if _, err = os.Stdout.Write(backups.GetBackupManifests()); err != nil {
			return fmt.Errorf("while printing the backup manifests: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	AdminCmd.AddCommand(listBackupsCmd)

	socketPath := path.Join(os.TempDir(), ".klio-admin")
	listBackupsCmd.Flags().String("socket-path", socketPath, "Unix socket used by the administration server")
}

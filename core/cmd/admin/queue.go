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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// queueCmd represents the queue command
//
//nolint:gochecknoglobals
var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Manage the queue tasks",
}

// queueStatusCmd represents the queue-status command
//
//nolint:gochecknoglobals
var queueStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the task queue (pending backups and pending WALs)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		socketPath, err := cmd.Flags().GetString("socket-path")
		if err != nil {
			return fmt.Errorf("while getting the socketPath flag: %w", err)
		}

		jsonOutput, err := cmd.Flags().GetBool("json")
		if err != nil {
			return fmt.Errorf("while getting the json flag: %w", err)
		}

		conn, err := connectToAdminServer(socketPath)
		if err != nil {
			return err
		}
		defer func() {
			_ = conn.Close()
		}()

		adminClient := klioGRPC.NewAdminClient(conn)
		status, err := adminClient.QueueStatus(cmd.Context(), &klioGRPC.QueueStatusRequest{})
		if err != nil {
			return fmt.Errorf("while calling queue-status entrypoint: %w", err)
		}

		if jsonOutput {
			output := map[string]uint64{
				"pending_backups": status.GetPendingBackups(),
				"pending_wals":    status.GetPendingWals(),
			}
			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("while marshalling JSON output: %w", err)
			}
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", data)
		} else {
			_, _ = fmt.Fprintf(os.Stdout, "Pending backups: %d\n", status.GetPendingBackups())
			_, _ = fmt.Fprintf(os.Stdout, "Pending WALs: %d\n", status.GetPendingWals())
		}

		return nil
	},
}

// failedTaskTarget selects the failed tasks a retry or discard command acts on.
// A nil cluster name selects every cluster, and empty WAL names select every
// WAL of the cluster.
type failedTaskTarget struct {
	clusterName *string
	walNames    []string
}

// failedTaskArgs validates the positional arguments of a retry or discard
// command: none are allowed with --all-clusters, otherwise withoutAllClusters
// applies.
func failedTaskArgs(withoutAllClusters cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		allClusters, err := cmd.Flags().GetBool("all-clusters")
		if err != nil {
			return fmt.Errorf("while getting the all-clusters flag: %w", err)
		}
		if allClusters {
			return cobra.NoArgs(cmd, args)
		}

		return withoutAllClusters(cmd, args)
	}
}

// runFailedTaskAction parses the target of a retry or discard command,
// connects to the administration server and runs action against it.
func runFailedTaskAction(
	cmd *cobra.Command,
	args []string,
	action func(ctx context.Context, client klioGRPC.AdminClient, target failedTaskTarget) error,
) error {
	socketPath, err := cmd.Flags().GetString("socket-path")
	if err != nil {
		return fmt.Errorf("while getting the socketPath flag: %w", err)
	}

	allClusters, err := cmd.Flags().GetBool("all-clusters")
	if err != nil {
		return fmt.Errorf("while getting the all-clusters flag: %w", err)
	}

	var target failedTaskTarget
	if !allClusters {
		clusterName := args[0]
		target.clusterName = &clusterName
		if len(args) > 1 {
			target.walNames = args[1:]
		}
	}

	conn, err := connectToAdminServer(socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	return action(cmd.Context(), klioGRPC.NewAdminClient(conn), target)
}

//nolint:gochecknoinits
func init() {
	AdminCmd.AddCommand(queueCmd)
	queueCmd.AddCommand(queueStatusCmd)

	socketPath := path.Join(os.TempDir(), ".klio-admin")
	queueCmd.PersistentFlags().String("socket-path", socketPath, "Unix socket used by the administration server")
	queueCmd.PersistentFlags().Bool("json", false, "Output in JSON format")
}

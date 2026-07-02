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
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// queueWALCmd represents the queue WAL command
//
//nolint:gochecknoglobals
var queueWALCmd = &cobra.Command{
	Use:   "wal",
	Short: "Manage the queue WAL tasks",
}

//nolint:gochecknoglobals
var listFailedWALCmd = &cobra.Command{
	Use:   "list-failed",
	Short: "List failed WAL tasks in the queue",
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

		clusterName, err := cmd.Flags().GetString("cluster-name")
		if err != nil {
			return fmt.Errorf("while getting the cluster-name flag: %w", err)
		}

		conn, err := connectToAdminServer(socketPath)
		if err != nil {
			return err
		}
		defer func() {
			_ = conn.Close()
		}()

		adminClient := klioGRPC.NewAdminClient(conn)
		response, err := adminClient.QueueListFailedWALs(cmd.Context(), &klioGRPC.QueueListFailedWALsRequest{
			ClusterName: &clusterName,
		})
		if err != nil {
			return fmt.Errorf("while calling queue list-failed wals entrypoint: %w", err)
		}

		if jsonOutput {
			data, err := protojson.MarshalOptions{
				Multiline:         true,
				EmitDefaultValues: true,
			}.Marshal(response)
			if err != nil {
				return fmt.Errorf("while marshalling JSON output: %w", err)
			}
			_, _ = fmt.Fprintf(os.Stdout, "%s\n", data)
		} else {
			rows := make([][]string, 0, len(response.GetWals()))
			for _, wal := range response.GetWals() {
				rows = append(rows, []string{
					strconv.FormatUint(wal.GetSequence(), 10),
					wal.GetClusterName(),
					wal.GetWalName(),
					wal.GetLastAttemptTime().AsTime().Format(time.RFC3339),
				})
			}
			if err := writeTable(
				os.Stdout,
				[]string{"SEQUENCE", "CLUSTER", "WAL NAME", "LAST ATTEMPT"},
				rows,
			); err != nil {
				return fmt.Errorf("while writing table output: %w", err)
			}
		}

		return nil
	},
}

//nolint:gochecknoglobals
var retryWALCmd = &cobra.Command{
	Use:   "retry [cluster-name] [WAL1 WAL2 ...]",
	Short: "Retry failed WAL tasks in the queue",
	Long: "Retry failed WAL tasks in the queue.\n\n" +
		"With no arguments, all failed WAL tasks are retried. If a cluster name " +
		"is given, all failed WAL tasks for that cluster are retried. If WAL " +
		"files are also given, only those are retried.",
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		var request klioGRPC.QueueRetryWALsRequest
		if len(args) > 0 {
			clusterName := args[0]
			request.ClusterName = &clusterName
		}
		if len(args) > 1 {
			request.WalNames = args[1:]
		}

		adminClient := klioGRPC.NewAdminClient(conn)
		_, err = adminClient.QueueRetryWALs(cmd.Context(), &request)
		if err != nil {
			return fmt.Errorf("while calling queue retry wals entrypoint: %w", err)
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	queueCmd.AddCommand(queueWALCmd)

	queueWALCmd.AddCommand(listFailedWALCmd)
	listFailedWALCmd.Flags().String("cluster-name", "", "Cluster name to filter failed WAL tasks (optional)")

	queueWALCmd.AddCommand(retryWALCmd)
}

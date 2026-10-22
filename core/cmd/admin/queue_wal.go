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
	"fmt"
	"os"
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
		jsonOutput, err := cmd.Flags().GetBool("json")
		if err != nil {
			return fmt.Errorf("while getting the json flag: %w", err)
		}

		clusterName, err := cmd.Flags().GetString("cluster-name")
		if err != nil {
			return fmt.Errorf("while getting the cluster-name flag: %w", err)
		}

		adminClient, closeConn, err := dialAdmin(cmd)
		if err != nil {
			return err
		}
		defer closeConn()

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
					wal.GetClusterName(),
					wal.GetWalName(),
					wal.GetLastAttemptTime().AsTime().Format(time.RFC3339),
				})
			}
			if err := writeTable(
				os.Stdout,
				[]string{"CLUSTER", "WAL NAME", "LAST ATTEMPT"},
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
		"A cluster name is required, and all failed WAL tasks for that cluster " +
		"are retried. If WAL files are also given, only those are retried. Pass " +
		"--all-clusters instead of a cluster name to retry all failed WAL tasks " +
		"across every cluster.",
	Args: failedTaskArgs(cobra.MinimumNArgs(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFailedTaskAction(cmd, args,
			func(ctx context.Context, client klioGRPC.AdminClient, target failedTaskTarget) error {
				if _, err := client.QueueRetryWALs(ctx, &klioGRPC.QueueRetryWALsRequest{
					ClusterName: target.clusterName,
					WalNames:    target.walNames,
				}); err != nil {
					return fmt.Errorf("while calling queue retry wals entrypoint: %w", err)
				}

				return nil
			})
	},
}

//nolint:gochecknoglobals
var discardWALCmd = &cobra.Command{
	Use:   "discard [cluster-name] [WAL1 WAL2 ...]",
	Short: "Discard failed WAL tasks in the queue",
	Long: "Discard failed WAL tasks in the queue.\n\n" +
		"A cluster name is required, and all failed WAL tasks for that cluster " +
		"are discarded. If WAL files are also given, only those are discarded. Pass " +
		"--all-clusters instead of a cluster name to discard all failed WAL tasks " +
		"across every cluster.\n\n" +
		"Discarding removes the failed task from the queue, not the WAL file " +
		"itself. The task is permanent: the WAL file is never archived to " +
		"Tier 2, which can leave a gap in the WAL archive and prevent " +
		"point-in-time recovery past that point. Use \"klio admin queue wal " +
		"list-failed\" to review the failed WAL tasks first.\n\n" +
		"The number of discarded tasks is printed when the command completes.",
	Args: failedTaskArgs(cobra.MinimumNArgs(1)),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFailedTaskAction(cmd, args,
			func(ctx context.Context, client klioGRPC.AdminClient, target failedTaskTarget) error {
				resp, err := client.QueueDiscardWALs(ctx, &klioGRPC.QueueDiscardWALsRequest{
					ClusterName: target.clusterName,
					WalNames:    target.walNames,
				})
				if err != nil {
					return fmt.Errorf("while calling queue discard wals entrypoint: %w", err)
				}

				return printDiscarded(cmd, resp.GetDiscarded(), "WAL")
			})
	},
}

//nolint:gochecknoinits
func init() {
	queueCmd.AddCommand(queueWALCmd)

	queueWALCmd.AddCommand(listFailedWALCmd)
	listFailedWALCmd.Flags().String("cluster-name", "", "Cluster name to filter failed WAL tasks (optional)")

	queueWALCmd.AddCommand(retryWALCmd)
	retryWALCmd.Flags().Bool("all-clusters", false, "Retry failed WAL tasks across every cluster")

	queueWALCmd.AddCommand(discardWALCmd)
	discardWALCmd.Flags().Bool("all-clusters", false, "Discard failed WAL tasks across every cluster")
}

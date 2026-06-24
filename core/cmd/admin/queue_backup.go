package admin

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// queueBackupCmd represents the queue backup command
//
//nolint:gochecknoglobals
var queueBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage the queue backup tasks",
}

//nolint:gochecknoglobals
var listFailedBackupCmd = &cobra.Command{
	Use:   "list-failed",
	Short: "List failed backup tasks in the queue",
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
		response, err := adminClient.QueueListFailedBackups(cmd.Context(), &klioGRPC.QueueListFailedBackupsRequest{
			ClusterName: &clusterName,
		})
		if err != nil {
			return fmt.Errorf("while calling queue list-failed backups entrypoint: %w", err)
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
			rows := make([][]string, 0, len(response.GetBackups()))
			for _, backup := range response.GetBackups() {
				rows = append(rows, []string{
					backup.GetClusterName(),
					backup.GetLastAttemptTime().AsTime().Format(time.RFC3339),
				})
			}
			if err := writeTable(os.Stdout, []string{"CLUSTER", "LAST ATTEMPT"}, rows); err != nil {
				return fmt.Errorf("while writing table output: %w", err)
			}
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	queueCmd.AddCommand(queueBackupCmd)
	queueBackupCmd.AddCommand(listFailedBackupCmd)

	listFailedBackupCmd.Flags().String("cluster-name", "", "Cluster name to filter failed backup tasks (optional)")
}

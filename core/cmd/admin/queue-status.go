package admin

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// queueStatusCmd represents the queue-status command
//
//nolint:gochecknoglobals
var queueStatusCmd = &cobra.Command{
	Use:   "queue-status",
	Short: "Show the status of the task queue (pending backups and pending WALs)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		socketPath, err := cmd.Flags().GetString("socketPath")
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

//nolint:gochecknoinits
func init() {
	AdminCmd.AddCommand(queueStatusCmd)

	socketPath := path.Join(os.TempDir(), ".klio-admin")
	queueStatusCmd.Flags().String("socketPath", socketPath, "Unix socket used by the administration server")
	queueStatusCmd.Flags().Bool("json", false, "Output in JSON format")
}

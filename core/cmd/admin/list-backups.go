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

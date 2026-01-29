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
		socketPath, err := cmd.Flags().GetString("socketPath")
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
	refreshCmd.Flags().String("socketPath", socketPath, "Unix socket used by the administration server")
}

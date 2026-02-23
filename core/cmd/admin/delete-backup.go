package admin

import (
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

// deleteBackupCmd represents the delete-backup command.
//
//nolint:gochecknoglobals
var deleteBackupCmd = &cobra.Command{
	Use:   "delete-backup [backupName]",
	Short: "Delete a backup from the Klio server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupName := args[0]

		socketPath, err := cmd.Flags().GetString("socketPath")
		if err != nil {
			return fmt.Errorf("while getting the socketPath flag: %w", err)
		}

		clusterName, err := cmd.Flags().GetString("cluster")
		if err != nil {
			return fmt.Errorf("while getting the cluster flag: %w", err)
		}

		tier1, err := cmd.Flags().GetBool("tier1")
		if err != nil {
			return fmt.Errorf("while getting the tier1 flag: %w", err)
		}

		tier2, err := cmd.Flags().GetBool("tier2")
		if err != nil {
			return fmt.Errorf("while getting the tier2 flag: %w", err)
		}

		tiers := buildTiersList(tier1, tier2)

		conn, err := connectToAdminServer(socketPath)
		if err != nil {
			return err
		}
		defer func() {
			_ = conn.Close()
		}()

		adminClient := klioGRPC.NewAdminClient(conn)
		_, err = adminClient.DeleteBackup(cmd.Context(), &klioGRPC.DeleteBackupRequest{
			BackupName:  backupName,
			Tiers:       tiers,
			ClusterName: clusterName,
		})
		if err != nil {
			return fmt.Errorf("while calling delete-backup entrypoint: %w", err)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Backup %q deleted successfully\n", backupName)

		return nil
	},
}

func buildTiersList(tier1, tier2 bool) []klioGRPC.Tier {
	var tiers []klioGRPC.Tier
	if tier1 {
		tiers = append(tiers, klioGRPC.Tier_TIER_1)
	}
	if tier2 {
		tiers = append(tiers, klioGRPC.Tier_TIER_2)
	}

	return tiers
}

//nolint:gochecknoinits
func init() {
	AdminCmd.AddCommand(deleteBackupCmd)

	socketPath := path.Join(os.TempDir(), ".klio-admin")
	deleteBackupCmd.Flags().String("socketPath", socketPath, "Unix socket used by the administration server")
	deleteBackupCmd.Flags().String("cluster", "", "The name of the cluster that owns the backup")
	deleteBackupCmd.Flags().Bool("tier1", false, "Delete the backup from tier1 (local cache)")
	deleteBackupCmd.Flags().Bool("tier2", false, "Delete the backup from tier2 (object storage)")
	_ = deleteBackupCmd.MarkFlagRequired("cluster")
	deleteBackupCmd.MarkFlagsOneRequired("tier1", "tier2")
}

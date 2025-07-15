// Package backup contains the implementation of the `klio backup` command.
package backup

import (
	"github.com/spf13/cobra"
)

// BackupCmd the `klio backup` command
//
//nolint:gochecknoglobals
var BackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage physical backups",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

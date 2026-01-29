package admin

import (
	"github.com/spf13/cobra"
)

// AdminCmd is the `klio admin` command
//
//nolint:gochecknoglobals
var AdminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Server administration commands",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

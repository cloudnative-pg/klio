package retention

import (
	"github.com/spf13/cobra"
)

// RetentionCmd the `klio backup` command
//
//nolint:gochecknoglobals
var RetentionCmd = &cobra.Command{
	Use:   "retention",
	Short: "Manage the retention policy",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

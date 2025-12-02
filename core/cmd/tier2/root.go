package tier2

import (
	"github.com/spf13/cobra"
)

// Tier2Cmd the `klio tier2` command
//
//nolint:gochecknoglobals
var Tier2Cmd = &cobra.Command{
	Use:   "tier2",
	Short: "Tier 2 management commands",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

package walplayer

import (
	"github.com/spf13/cobra"
)

// WalPlayerCmd is the `klio wal-player` command
//
//nolint:gochecknoglobals
var WalPlayerCmd = &cobra.Command{
	Use:   "wal-player",
	Short: "WAL Player Commands",
}

// Package server contains the implementation of the `klio server` command.
package server

import (
	"github.com/spf13/cobra"
)

// ServerCmd the `klio server` command
//
//nolint:gochecknoglobals
var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts and manage a Klio server",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

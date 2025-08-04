package cnpgi

import (
	"github.com/spf13/cobra"
)

// CnpgiCmd is the `klio cnpgi` command
//
//nolint:gochecknoglobals
var CnpgiCmd = &cobra.Command{
	Use:    "cnpgi",
	Short:  "CNPG-I integration",
	Hidden: true,
}

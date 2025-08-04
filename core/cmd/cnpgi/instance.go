package cnpgi

import (
	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
)

// instanceCmd represents the `klio cnpgi instance` command
//
//nolint:gochecknoglobals
var instanceCmd = &cobra.Command{
	Use:    "instance",
	Short:  "Start the instance CNPG-I server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		pluginPath, _ := cmd.Flags().GetString("plugin-path")

		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability()
			server.AddMetricsCapability()
			server.AddWALCapability(true)
		}

		return runCNPGI(cmd.Context(), pluginPath, capabilities)
	},
}

//nolint:gochecknoinits
func init() {
	instanceCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)

	CnpgiCmd.AddCommand(instanceCmd)
}

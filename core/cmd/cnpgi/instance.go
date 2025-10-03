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
		metricsAddr, _ := cmd.Flags().GetString("metrics-bind-address")
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability()
			server.AddMetricsCapability()
			server.AddWALCapability(true)
		}

		return runCNPGI(cmd.Context(), pluginPath, metricsAddr, capabilities)
	},
}

//nolint:gochecknoinits
func init() {
	instanceCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)
	instanceCmd.Flags().String(
		"metrics-bind-address",
		":8081",
		"The address the metric endpoint binds to.",
	)

	CnpgiCmd.AddCommand(instanceCmd)
}

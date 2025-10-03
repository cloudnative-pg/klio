package cnpgi

import (
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
)

// restoreJobCmd represents the job run command
//
//nolint:gochecknoglobals
var restoreJobCmd = &cobra.Command{
	Use:    "restore [backup-name] [destination]",
	Short:  "Start the instance CNPG-I job restore server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginPath, _ := cmd.Flags().GetString("plugin-path")
		metricsAddress, _ := cmd.Flags().GetString("metrics-bind-address")
		contextLogger := log.FromContext(cmd.Context())
		contextLogger.Info("Starting CNPG-I job restore server",
			"pluginPath", pluginPath,
			"backupName", args[0],
			"destination", args[1],
			"metricsAddress", metricsAddress,
		)
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddRestoreCapability(args[0], args[1])
			server.AddWALCapability(true)
		}

		return runCNPGI(cmd.Context(), pluginPath, metricsAddress, capabilities)
	},
}

//nolint:gochecknoinits
func init() {
	restoreJobCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)
	restoreJobCmd.Flags().String(
		"metrics-bind-address",
		":8083",
		"The address the metric endpoint binds to.",
	)
	CnpgiCmd.AddCommand(restoreJobCmd)
}

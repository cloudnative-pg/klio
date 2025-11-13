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
	Use:    "restore [destination]",
	Short:  "Start the instance CNPG-I job restore server",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginPath, _ := cmd.Flags().GetString("plugin-path")
		destination := args[0]

		contextLogger := log.FromContext(cmd.Context())
		contextLogger.Info("Starting CNPG-I job restore server",
			"pluginPath", pluginPath,
			"destination", args[0],
		)
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddRestoreCapability(destination)
			server.AddWALCapability(true)
		}

		return runCNPGI(cmd.Context(), pluginPath, capabilities)
	},
}

//nolint:gochecknoinits
func init() {
	restoreJobCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)
	CnpgiCmd.AddCommand(restoreJobCmd)
}

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
		contextLogger := log.FromContext(cmd.Context())
		contextLogger.Info("Starting CNPG-I job restore server",
			"pluginPath", pluginPath,
			"backupName", args[0],
			"destination", args[1],
		)
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddRestoreCapability(args[0], args[1])
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

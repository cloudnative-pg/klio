package cnpgi

import (
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"

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
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")
		tier1, _ := cmd.Flags().GetBool("tier1")
		tier2, _ := cmd.Flags().GetBool("tier2")
		debug, _ := cmd.PersistentFlags().GetBool("debug")
		destination := args[0]

		contextLogger := log.FromContext(cmd.Context())
		contextLogger.Info("Starting CNPG-I job restore server",
			"pluginPath", pluginPath,
			"destination", args[0],
		)
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddRestoreCapability(destination)
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug: debug,
				Tier1: tier1,
				Tier2: tier2,
			})
		}

		return runCNPGI(
			cmd.Context(),
			pluginPath,
			types.NamespacedName{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			capabilities,
			nil,
		)
	},
}

//nolint:gochecknoinits
func init() {
	restoreJobCmd.Flags().String(
		"cluster-name",
		"",
		"The name of the cluster object",
	)
	restoreJobCmd.Flags().String(
		"cluster-namespace",
		"",
		"The namespace of the cluster object",
	)
	restoreJobCmd.Flags().String(
		"plugin-path",
		"/plugins",
		"The directory where the Unix domain socket should be created",
	)
	restoreJobCmd.Flags().Bool(
		"tier1",
		true,
		"If enabled, look for backup and WALs in tier1",
	)
	restoreJobCmd.Flags().Bool(
		"tier2",
		false,
		"If enabled, look for backup and WALs in tier2",
	)
	CnpgiCmd.AddCommand(restoreJobCmd)
}

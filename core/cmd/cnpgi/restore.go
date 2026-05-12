package cnpgi

import (
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
	"github.com/cloudnative-pg/klio/core/pkg/config"
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
		configFile, _ := cmd.Root().PersistentFlags().GetString("config")
		pluginPath, _ := cmd.Flags().GetString("plugin-path")
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")
		debug, _ := cmd.PersistentFlags().GetBool("debug")
		destination := args[0]

		var configuration config.Data
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		contextLogger := log.FromContext(cmd.Context())
		contextLogger.Info("Starting CNPG-I job restore server",
			"pluginPath", pluginPath,
			"destination", args[0],
		)
		capabilities := func(server *cnpgi.CNPGI) {
			server.AddRestoreCapability(destination)
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug: debug,
			})
		}

		return runCNPGI(
			cmd.Context(),
			pluginPath,
			configFile,
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
	CnpgiCmd.AddCommand(restoreJobCmd)
}

package cnpgi

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// instanceCmd represents the `klio cnpgi instance` command
//
//nolint:gochecknoglobals
var instanceCmd = &cobra.Command{
	Use:    "instance",
	Short:  "Start the instance CNPG-I server",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		configFile, _ := cmd.Root().PersistentFlags().GetString("config")
		pluginPath, _ := cmd.Flags().GetString("plugin-path")

		var configuration config.Data
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		debug, _ := cmd.PersistentFlags().GetBool("debug")
		podName, _ := cmd.Flags().GetString("pod-name")
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")

		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability(cnpgi.BackupCapabilityOptions{
				Tier2: configuration.Tier2BackupEnabled,
			})
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug: debug,
				Tier1: configuration.Tier1Enabled,
				Tier2: configuration.Tier2RecoveryEnabled,
			})
		}

		enrichManager := func(mgr manager.Manager) error {
			sendWal := cnpgi.SendWalClusterReconciler{
				Client:         mgr.GetClient(),
				PodName:        podName,
				KlioConfigFile: configFile,
			}

			return sendWal.SetupWithManager(mgr)
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
			enrichManager,
		)
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
		"cluster-name",
		"",
		"The name of the cluster object",
	)
	instanceCmd.Flags().String(
		"cluster-namespace",
		"",
		"The namespace of the cluster object",
	)
	instanceCmd.Flags().String(
		"pod-name",
		"",
		"The name of the current instance",
	)

	CnpgiCmd.AddCommand(instanceCmd)
}

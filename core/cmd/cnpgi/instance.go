package cnpgi

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

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
		configFile, _ := cmd.Root().PersistentFlags().GetString("config")
		pluginPath, _ := cmd.Flags().GetString("plugin-path")

		tier1, _ := cmd.Flags().GetBool("tier1")
		enableTier2Backup, _ := cmd.Flags().GetBool("enable-tier2-backup")
		enableTier2Recovery, _ := cmd.Flags().GetBool("enable-tier2-recovery")
		debug, _ := cmd.PersistentFlags().GetBool("debug")
		podName, _ := cmd.Flags().GetString("pod-name")
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")

		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability(cnpgi.BackupCapabilityOptions{
				Tier2: enableTier2Backup,
			})
			server.AddMetricsCapability()
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug: debug,
				Tier1: tier1,
				Tier2: enableTier2Recovery,
			})
		}

		enrichManager := func(mgr manager.Manager) error {
			sendWal := cnpgi.SendWalClusterReconciler{
				Client:            mgr.GetClient(),
				PodName:           podName,
				KlioConfigFile:    configFile,
				EnableTier2Backup: enableTier2Backup,
			}

			return sendWal.SetupWithManager(mgr)
		}

		return runCNPGI(
			cmd.Context(),
			pluginPath,
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
	instanceCmd.Flags().Bool(
		"tier1",
		true,
		"Enabled when the cluster is connected to tier1",
	)
	instanceCmd.Flags().Bool(
		"enable-tier2-backup",
		false,
		"Enabled when backups need to be sent to tier2",
	)
	instanceCmd.Flags().Bool(
		"enable-tier2-recovery",
		false,
		"Enabled when WALs need to be read from both tier1 and tier2",
	)

	CnpgiCmd.AddCommand(instanceCmd)
}

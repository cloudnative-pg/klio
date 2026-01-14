package cnpgi

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/cnpgi"
)

// sendWalCmd represents the `klio cnpgi send-wal` command
//
//nolint:gochecknoglobals
var sendWalCmd = &cobra.Command{
	Use:    "send-wal",
	Short:  "Sends WALs to a Klio server when needed",
	Hidden: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		configFile, _ := cmd.Root().PersistentFlags().GetString("config")
		clusterName, _ := cmd.Flags().GetString("cluster-name")
		clusterNamespace, _ := cmd.Flags().GetString("cluster-namespace")
		podName, _ := cmd.Flags().GetString("pod-name")
		enableTier2Backup, _ := cmd.Flags().GetBool("enable-tier2-backup")

		_ = viper.BindEnv("custom-cnpg-group", "CUSTOM_CNPG_GROUP")
		_ = viper.BindEnv("custom-cnpg-version", "CUSTOM_CNPG_VERSION")

		ctrl := cnpgi.SendWalController{
			Scheme:         generateScheme(),
			KlioConfigFile: configFile,
			ClusterKey: types.NamespacedName{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			PodName: podName,
			Tier2:   enableTier2Backup,
		}

		return ctrl.Start(cmd.Context())
	},
}

//nolint:gochecknoinits
func init() {
	sendWalCmd.Flags().String(
		"cluster-name",
		"",
		"The name of the cluster object",
	)
	sendWalCmd.Flags().String(
		"cluster-namespace",
		"",
		"The namespace of the cluster object",
	)
	sendWalCmd.Flags().String(
		"pod-name",
		"",
		"The name of the current instance",
	)
	sendWalCmd.Flags().Bool(
		"enable-tier2-backup",
		false,
		"Send WALs to tier2",
	)

	CnpgiCmd.AddCommand(sendWalCmd)
}

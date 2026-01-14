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

		enableTier2Backup, _ := cmd.Flags().GetBool("enable-tier2-backup")
		enableTier2Recovery, _ := cmd.Flags().GetBool("enable-tier2-recovery")

		capabilities := func(server *cnpgi.CNPGI) {
			server.AddBackupCapability(cnpgi.BackupCapabilityOptions{
				Tier2: enableTier2Backup,
			})
			server.AddMetricsCapability()
			server.AddWALCapability(cnpgi.WALCapabilityOptions{
				Debug:        true,
				IncludeTier2: enableTier2Recovery,
			})
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

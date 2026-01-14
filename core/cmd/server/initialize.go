package server

import (
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudnative-pg/klio/core/cmd/initialize"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// initializeCmd represents the "init" command
//
//nolint:gochecknoglobals
var initializeCmd = &cobra.Command{
	Use:   "initialize",
	Short: "Initialize a new Klio repository on the configured folder",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		var configuration config.ServerConfig

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if err := configuration.RequireTier1(); err != nil {
			return fmt.Errorf("tier 1 configuration validation error: %w", err)
		}

		skipIfExisting, _ := cmd.Flags().GetBool("skip-if-existing")
		walDirectory := configuration.Tier1.Wal.WALPath
		kopiaDirectory := configuration.Tier1.Base.RepositoryDirectory

		opts := initialize.Options{
			WalFS:                 afero.NewBasePathFs(afero.NewOsFs(), walDirectory),
			WalEncryptionPassword: configuration.Tier1.EncryptionKey,

			KopiaFS:                 afero.NewBasePathFs(afero.NewOsFs(), kopiaDirectory),
			KopiaEncryptionPassword: configuration.Tier1.EncryptionKey,
			KopiaInitializeRepo: func() error {
				return kopiaserver.InitializeTier1(ctx, &configuration.Tier1)
			},

			SkipIfExisting: skipIfExisting,
		}

		return initialize.Run(ctx, opts)
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(initializeCmd)

	// Here you will define your flags and configuration settings.
	initializeCmd.Flags().Bool(
		"skip-if-existing",
		false,
		"Skip initialization if the target directories already exist and are not empty",
	)

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

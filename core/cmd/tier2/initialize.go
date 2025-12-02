package tier2

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/cmd/initialize"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// initializeCmd represents the "init" command
//
//nolint:gochecknoglobals
var initializeCmd = &cobra.Command{
	Use:   "initialize",
	Short: "Initialize a new Tier2 repository",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		ctx := cmd.Context()

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		if !configuration.Tier2.S3.Enabled {
			// Nothing to be done. There is no tier2.
			return nil
		}

		skipIfExisting, _ := cmd.Flags().GetBool("skip-if-existing")

		tier2BaseFS, err := tier2.ConnectBase(cmd.Context(), &configuration.Tier2)
		if err != nil {
			return fmt.Errorf("error while connecting to tier2 (base): %w", err)
		}

		tier2WALFS, err := tier2.ConnectWAL(cmd.Context(), &configuration.Tier2)
		if err != nil {
			return fmt.Errorf("error while connecting to tier2 (wal): %w", err)
		}

		opts := initialize.Options{
			WalFS:                 tier2WALFS,
			WalEncryptionPassword: configuration.Tier2.S3.EncryptionPassword,

			KopiaFS:                 tier2BaseFS,
			KopiaEncryptionPassword: configuration.Tier2.S3.EncryptionPassword,
			KopiaInitializeRepo: func() error {
				return kopiaserver.InitializeTier2(ctx, &configuration.Tier2.S3)
			},

			SkipIfExisting: skipIfExisting,
		}

		return initialize.Run(ctx, opts)
	},
}

//nolint:gochecknoinits
func init() {
	Tier2Cmd.AddCommand(initializeCmd)

	// Here you will define your flags and configuration settings.
	initializeCmd.Flags().Bool(
		"skip-if-existing",
		false,
		"Skip initialization if the target directories already exist and are not empty",
	)
}

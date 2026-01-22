package server

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
	Short: "Initialize a new Klio repository on the configured folder",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.ServerConfig

		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if err := configuration.RequireTier1(); err != nil {
			return fmt.Errorf("tier 1 configuration validation error: %w", err)
		}

		skipIfExisting, _ := cmd.Flags().GetBool("skip-if-existing")
		enableTier1, _ := cmd.Flags().GetBool("tier1")
		enableTier2, _ := cmd.Flags().GetBool("tier2")

		if enableTier1 {
			if err := initializeTier1(cmd.Context(), &configuration, skipIfExisting); err != nil {
				return err
			}
		}

		if enableTier2 {
			if err := configuration.RequireTier2(); err != nil {
				return fmt.Errorf("tier 2 configuration validation error: %w", err)
			}

			if !configuration.Tier2.S3.Enabled {
				// Nothing to be done. There is no tier2.
				return nil
			}

			if err := initializeTier2(cmd.Context(), &configuration, skipIfExisting); err != nil {
				return err
			}
		}

		return nil
	},
}

func initializeTier1(ctx context.Context, cfg *config.ServerConfig, skipIfExisting bool) error {
	walDirectory := cfg.Tier1.Wal.WALPath
	kopiaDirectory := cfg.Tier1.Base.RepositoryDirectory

	opts := initialize.Options{
		WalFS:                 afero.NewBasePathFs(afero.NewOsFs(), walDirectory),
		WalEncryptionPassword: cfg.Tier1.EncryptionKey,

		KopiaFS:                 afero.NewBasePathFs(afero.NewOsFs(), kopiaDirectory),
		KopiaEncryptionPassword: cfg.Tier1.EncryptionKey,
		KopiaInitializeRepo: func() error {
			return kopiaserver.InitializeTier1(ctx, &cfg.Tier1)
		},

		SkipIfExisting: skipIfExisting,
	}

	log.FromContext(ctx).Info(
		"Ensuring tier1 repository is initialized.",
		"walDirectory", walDirectory,
		"kopiaDirectory", kopiaDirectory,
	)

	return initialize.Run(ctx, opts)
}

func initializeTier2(ctx context.Context, cfg *config.ServerConfig, skipIfExisting bool) error {
	tier2BaseFS, err := tier2.ConnectBase(ctx, &cfg.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2 (base): %w", err)
	}

	tier2WALFS, err := tier2.ConnectWAL(ctx, &cfg.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2 (wal): %w", err)
	}

	opts := initialize.Options{
		WalFS:                 tier2WALFS,
		WalEncryptionPassword: cfg.Tier2.EncryptionKey,

		KopiaFS:                 tier2BaseFS,
		KopiaEncryptionPassword: cfg.Tier2.EncryptionKey,
		KopiaInitializeRepo: func() error {
			return kopiaserver.InitializeTier2(ctx, &cfg.Tier2)
		},

		SkipIfExisting: skipIfExisting,
	}

	log.FromContext(ctx).Info("Ensuring tier2 repository is initialized.")

	return initialize.Run(ctx, opts)
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(initializeCmd)

	initializeCmd.Flags().Bool("tier1", true, "Enables Tier1 initialization")
	initializeCmd.Flags().Bool("tier2", false, "Enables Tier2 initialization")
	initializeCmd.Flags().Bool(
		"skip-if-existing",
		false,
		"Skip initialization if the target directories already exist and are not empty",
	)
}

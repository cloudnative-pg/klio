package server

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
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

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		walDirectory := configuration.Wal.WALPath
		kopiaDirectory := configuration.Base.RepositoryDirectory

		if _, err := os.Stat(walDirectory); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("while checking if the Klio WAL directory %q exists, %w", walDirectory, err)
			}

			if err := repository.Initialize(repository.Options{
				Path:     configuration.Wal.WALPath,
				Password: configuration.Wal.EncryptionPassword,
			}); err != nil {
				return fmt.Errorf("while initializing the Klio WAL directory %q, %w", walDirectory, err)
			}
		}

		if _, err := os.Stat(kopiaDirectory); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("while checking if the Kopia repository directory %q exists, %w", walDirectory, err)
			}

			if err := kopiaserver.Initialize(cmd.Context(), kopiaserver.InitOptions{
				Path:     configuration.Base.RepositoryDirectory,
				Password: configuration.Base.EncryptionPassword,
			}); err != nil {
				return fmt.Errorf("while initializing the Kopia repository directory %q, %w", walDirectory, err)
			}
		}

		return nil
	},
}

//nolint:gochecknoinits
func init() {
	ServerCmd.AddCommand(initializeCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

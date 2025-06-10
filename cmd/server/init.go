package server

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/validator.v2"

	"github.com/EnterpriseDB/klio/internal/server/kopiaserver"
	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// initializeCmd represents the "init" command
//
//nolint:gochecknoglobals
var initializeCmd = &cobra.Command{
	Use:   "initialize",
	Short: "Initialize a new Klio repository on the configured folder",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var configuration config.Data

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		// Sets the defaults values, to be overridden by the user configuration
		configuration.SetDefaults()

		if configuration.Server == nil {
			return ErrKlioServerSectionIsRequired
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		walDirectory := configuration.Server.Klio.WALPath
		kopiaDirectory := configuration.Server.Kopia.RepositoryDirectory

		if _, err := os.Stat(walDirectory); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("while checking if the Klio WAL directory %q exists, %w", walDirectory, err)
			}

			if err := repository.Initialize(repository.Options{
				Path:     configuration.Server.Klio.WALPath,
				Password: configuration.Server.Klio.EncryptionPassword,
			}); err != nil {
				return fmt.Errorf("while initializing the Klio WAL directory %q, %w", walDirectory, err)
			}
		}

		if _, err := os.Stat(kopiaDirectory); err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("while checking if the Kopia repository directory %q exists, %w", walDirectory, err)
			}

			if err := kopiaserver.Initialize(cmd.Context(), kopiaserver.InitOptions{
				Path:     configuration.Server.Kopia.RepositoryDirectory,
				Password: configuration.Server.Kopia.EncryptionPassword,
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

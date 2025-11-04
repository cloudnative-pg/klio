package server

import (
	"fmt"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
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

		contextLogger := log.FromContext(cmd.Context())

		// IMPORTANT: this requires this program to be built with "-tags viper_bind_struct"
		// when using environment variables
		if err := viper.Unmarshal(&configuration); err != nil {
			return fmt.Errorf("could not unmarshal configuration: %w", err)
		}

		if errs := validator.Validate(&configuration); errs != nil {
			return fmt.Errorf("configuration validation error: %w", errs)
		}

		skipIfExisting, _ := cmd.Flags().GetBool("skip-if-existing")
		walDirectory := configuration.Wal.WALPath
		kopiaDirectory := configuration.Base.RepositoryDirectory

		osFS := afero.NewOsFs()

		var walDirectoryIsOk, kopiaDirectoryIsOk bool
		var err error

		walDirectoryIsOk, err = canInitRepoDirectory(osFS, walDirectory)
		if err != nil {
			return fmt.Errorf("while checking if the Klio WAL directory %q is safe to use: %w", walDirectory, err)
		}

		kopiaDirectoryIsOk, err = canInitRepoDirectory(osFS, kopiaDirectory)
		if err != nil {
			return fmt.Errorf("while checking if the Kopia repository %q is safe to use: %w", kopiaDirectory, err)
		}

		switch {
		case walDirectoryIsOk && kopiaDirectoryIsOk:
			walFS := afero.NewBasePathFs(afero.NewOsFs(), configuration.Wal.WALPath)
			if err := repository.Initialize(repository.Options{
				FS:       walFS,
				Password: configuration.Wal.EncryptionPassword,
			}); err != nil {
				return fmt.Errorf("while initializing the Klio WAL directory %q, %w", walDirectory, err)
			}

			if err := kopiaserver.Initialize(cmd.Context(), kopiaserver.InitOptions{
				Path:     configuration.Base.RepositoryDirectory,
				Password: configuration.Base.EncryptionPassword,
			}); err != nil {
				return fmt.Errorf("while initializing the Kopia repository directory %q, %w", walDirectory, err)
			}

		case skipIfExisting:
			contextLogger.Info(
				"skipping initialization of Klio repository directories",
				"walDirectory", walDirectory,
				"kopiaDirectory", kopiaDirectory,
			)

		case !walDirectoryIsOk:
			return fmt.Errorf("cannot initialize %q as Klio WAL directory because it is not empty", walDirectory)

		case !kopiaDirectoryIsOk:
			return fmt.Errorf("cannot initialize %q as Kopia repository because it is not empty", kopiaDirectory)
		}

		return nil
	},
}

// canInitRepoDirectory checks whether a directory does not exist or is empty
// and can be used to create a new repository.
func canInitRepoDirectory(fs afero.Fs, name string) (bool, error) {
	entries, err := afero.ReadDir(fs, name)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return len(entries) == 0, nil
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

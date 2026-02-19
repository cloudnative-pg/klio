package repository

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
)

const repositoryConfigFileName = "repository.config"

// Options are the initialization options for a
// Klio repository.
type Options struct {
	FS       afero.Fs
	Password string //nolint:gosec
}

// FileExists checks if a file exists in a Afero-based FS.
func FileExists(fs afero.Fs, fileName string) (bool, error) {
	if _, err := fs.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// Initialize initializes a new Klio repository.
func Initialize(options Options) error {
	if err := options.FS.MkdirAll(".", 0o750); err != nil {
		return fmt.Errorf("while ensuring that the repository directory exists: %w", err)
	}

	configExisting, err := FileExists(options.FS, repositoryConfigFileName)
	if err != nil {
		return fmt.Errorf("while checking config file %s existence: %w", repositoryConfigFileName, err)
	}
	if configExisting {
		log.Debug("stopping the repository initialization, configuration already exists")
		return nil
	}

	config, err := createNewRepositoryConfiguration(options.Password)
	if err != nil {
		return fmt.Errorf("while creating repository configuration: %w", err)
	}

	file, err := options.FS.OpenFile(repositoryConfigFileName, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("while creating config file %s: %w", repositoryConfigFileName, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if err = json.NewEncoder(file).Encode(config); err != nil {
		return fmt.Errorf("while encoding JSON configuration: %w", err)
	}

	return nil
}

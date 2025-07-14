package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/cloudnative-pg/machinery/pkg/fileutils"
	"github.com/cloudnative-pg/machinery/pkg/log"
)

const repositoryConfigFileName = "repository.config"

// Options are the initialization options for a
// Klio repository.
type Options struct {
	Path     string
	Password string
}

// Initialize initializes a new Klio repository.
func Initialize(options Options) error {
	if err := os.MkdirAll(options.Path, 0o750); err != nil {
		return fmt.Errorf("while ensuring that %s is a directory: %w", options.Path, err)
	}

	configFilePath := path.Join(options.Path, repositoryConfigFileName)
	configExisting, err := fileutils.FileExists(configFilePath)
	if err != nil {
		return fmt.Errorf("while checking config file %s existence: %w", configFilePath, err)
	}
	if configExisting {
		log.Debug("stopping the repository initialization, configuration already exists", "path", options.Path)
		return nil
	}

	config, err := createNewRepositoryConfiguration(options.Password)
	if err != nil {
		return fmt.Errorf("while creating repository configuration: %w", err)
	}

	file, err := os.OpenFile(configFilePath, os.O_WRONLY|os.O_CREATE, 0o600) //nolint:gosec
	if err != nil {
		return fmt.Errorf("while creating config file %s: %w", configFilePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if err = json.NewEncoder(file).Encode(config); err != nil {
		return fmt.Errorf("while encoding JSON configuration: %w", err)
	}

	return nil
}

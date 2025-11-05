package repository

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/afero"
)

// Connection represent a local connection to a Klio repository.
type Connection struct {
	config *KlioRepositoryConfig

	fs afero.Fs

	//nolint:godox
	// TODO(leonardoce)
	// Should we really keep the master key in our memory? Or should we seal it?
	// For now, let's start with this implementation and evaluate it later on.
	// The main points are:
	//
	// 1. for how much time this structure will be kept in memory?
	// 2. how much time is needed to decode the master key?
	masterKey []byte
}

// Open opens a connection to a repository.
func Open(options Options) (*Connection, error) {
	configFile, err := options.FS.OpenFile(repositoryConfigFileName, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("while opening configuration file %s: %w", repositoryConfigFileName, err)
	}
	defer func() {
		_ = configFile.Close()
	}()

	var config KlioRepositoryConfig
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		return nil, fmt.Errorf("while decoding JSON from configuration file %s: %w", repositoryConfigFileName, err)
	}

	masterKey, err := config.RecoverMasterKey(options.Password)
	if err != nil {
		return nil, fmt.Errorf("while recovering the master key: %w", err)
	}

	return &Connection{
		config:    &config,
		masterKey: masterKey,
		fs:        options.FS,
	}, nil
}

// Close closes the connection to the repository.
func (*Connection) Close() {}

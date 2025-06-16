package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

// Connection represent a local connection to a Klio repository.
type Connection struct {
	config *KlioRepositoryConfig

	baseDir string

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
	configFilePath := path.Join(options.Path, repositoryConfigFileName)

	configFile, err := os.OpenFile(configFilePath, os.O_RDONLY, 0) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening configuration file %s: %w", configFilePath, err)
	}
	defer func() {
		_ = configFile.Close()
	}()

	var config KlioRepositoryConfig
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		return nil, fmt.Errorf("while decoding JSON from configuration file %s: %w", configFilePath, err)
	}

	masterKey, err := config.RecoverMasterKey(options.Password)
	if err != nil {
		return nil, fmt.Errorf("while recovering the master key: %w", err)
	}

	return &Connection{
		config:    &config,
		masterKey: masterKey,
		baseDir:   options.Path,
	}, nil
}

// Close closes the connection to the repository.
func (*Connection) Close() {}

// BaseDir returns the base directory of this repository.
func (c *Connection) BaseDir() string {
	return c.baseDir
}

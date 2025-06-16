package kopia

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
)

// TemporaryConnection is a connection to a temporary repository, to be
// deleted after the client is closed.
type TemporaryConnection struct {
	Connection
	options LocalRepositoryOptions
}

// Close closes the connection to the repository.
func (s *TemporaryConnection) Close(ctx context.Context) error {
	if err := s.Connection.Close(ctx); err != nil {
		return err
	}

	if err := os.RemoveAll(s.options.Path); err != nil {
		return fmt.Errorf("while cleaning up directory %s: %w", s.options.Path, err)
	}

	return nil
}

// ConnectTemporary creates a connection to a local Kopia repository, creating it
// if not initialized.
func ConnectTemporary(
	ctx context.Context,
	logger *slog.Logger,
	options LocalRepositoryOptions,
) (*TemporaryConnection, error) {
	fsStorage, err := filesystem.New(
		ctx,
		&filesystem.Options{
			Path: options.Path,
		},
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("while creating storage interface: %w", err)
	}

	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}
	defer func() {
		err := os.Remove(configFile.Name())
		if err != nil {
			logger.Error("Error while removing temporary config file", "configFile", configFile.Name(), "err", err)
		}
	}()

	if options.Initialize {
		if err := repo.Initialize(ctx, fsStorage, &repo.NewRepositoryOptions{}, options.Password); err != nil {
			return nil, fmt.Errorf("while initializing repository: %w", err)
		}
	}

	if err := repo.Connect(ctx, configFile.Name(), fsStorage, options.Password, &repo.ConnectOptions{
		ClientOptions: repo.ClientOptions{
			Hostname: options.Hostname,
			Username: options.Username,
		},
	}); err != nil {
		return nil, fmt.Errorf("while connecting to repository: %w", err)
	}

	// Opens a connection to the repository using the persisted configuration file
	repository, err := repo.Open(ctx, configFile.Name(), options.Password, &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("while opening the repository: %w", err)
	}

	return &TemporaryConnection{
		options: options,
		Connection: Connection{
			logger:     logger,
			hostname:   options.Hostname,
			username:   options.Username,
			repository: repository,
		},
	}, nil
}

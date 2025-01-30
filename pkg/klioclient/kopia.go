package klioclient

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"

	"github.com/EnterpriseDB/klio/pkg/config"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	logger     *slog.Logger
	repository repo.Repository
	hostname   string
	username   string
}

// LocalRepositoryOptions are the options needed to create a local Kopia repository.
type LocalRepositoryOptions struct {
	Path       string
	Password   string
	Hostname   string
	Username   string
	Initialize bool
}

// Connect creates a new Kopia client and opens a connection to it.
func Connect(
	ctx context.Context,
	logger *slog.Logger,
	serverConfig *config.Server,
) (*Connection, error) {
	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	if err = repo.ConnectAPIServer(
		ctx,
		configFile.Name(),
		&repo.APIServerInfo{
			BaseURL:                             serverConfig.BaseURL,
			TrustedServerCertificateFingerprint: serverConfig.TrustedServerCertificateFingerprint,
		},
		serverConfig.Password,
		&repo.ConnectOptions{
			ClientOptions: repo.ClientOptions{
				Hostname: serverConfig.Hostname,
				Username: serverConfig.Username,
			},
		},
	); err != nil {
		return nil, fmt.Errorf("while pinging the repository: %w", err)
	}

	repository, err := repo.Open(ctx, configFile.Name(), serverConfig.Password, &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("while opening the repository: %w", err)
	}

	return &Connection{
		logger:     logger,
		hostname:   serverConfig.Hostname,
		username:   serverConfig.Username,
		repository: repository,
	}, nil
}

// ConnectLocal creates a connection to a local Kopia repository, creating it
// if not initialized.
func ConnectLocal(
	ctx context.Context,
	logger *slog.Logger,
	options LocalRepositoryOptions,
) (*Connection, error) {
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

	return &Connection{
		logger:     logger,
		hostname:   options.Hostname,
		username:   options.Username,
		repository: repository,
	}, nil
}

// Close closes the connection to the repository.
func (s *Connection) Close(ctx context.Context) error {
	return fmt.Errorf("while closing connection to Klio server: %w", s.repository.Close(ctx))
}

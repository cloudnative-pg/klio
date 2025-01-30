package klioclient

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kopia/kopia/repo"

	"github.com/EnterpriseDB/klio/pkg/config"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	logger       *slog.Logger
	repository   repo.Repository
	serverConfig *config.Server
}

// Connect creates a new Kopia client and opens a connection to it.
func Connect(
	ctx context.Context,
	logger *slog.Logger,
	serverConfig *config.Server,
) (*Connection, error) {
	result := &Connection{
		logger:       logger,
		serverConfig: serverConfig,
	}

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

	result.repository = repository

	return result, nil
}

// Close closes the connection to the repository.
func (s *Connection) Close(ctx context.Context) error {
	return fmt.Errorf("while closing connection to Klio server: %w", s.repository.Close(ctx))
}

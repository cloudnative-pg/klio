package kopia

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kopia/kopia/repo"

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
	kopiaClientConfig *config.KopiaRepositoryClientConfig,
) (*Connection, error) {
	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	certificateFingerprint := kopiaClientConfig.TrustedServerCertificateFingerprint
	if len(kopiaClientConfig.ServerCertPath) > 0 {
		var err error
		certificateFingerprint, err = extractSHA256CertificateFingerprint(kopiaClientConfig.ServerCertPath)
		if err != nil {
			return nil, fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
		}
	}

	// Normalize the hostname by eventually removing
	// the '@<hostname>' suffix, which is already been
	// appended by Kopia.
	//
	// We should have a debate about
	// this. Should we really do it? Should we just
	// drop the suffix?
	hostName := kopiaClientConfig.Hostname
	userName := strings.TrimSuffix(kopiaClientConfig.Username, "@"+hostName)

	if err = repo.ConnectAPIServer(
		ctx,
		configFile.Name(),
		&repo.APIServerInfo{
			BaseURL:                             kopiaClientConfig.BaseURL,
			TrustedServerCertificateFingerprint: certificateFingerprint,
		},
		kopiaClientConfig.Password,
		&repo.ConnectOptions{
			ClientOptions: repo.ClientOptions{
				Hostname: hostName,
				Username: userName,
			},
		},
	); err != nil {
		return nil, fmt.Errorf("while pinging the repository: %w", err)
	}

	repository, err := repo.Open(ctx, configFile.Name(), kopiaClientConfig.Password, &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("while opening the repository: %w", err)
	}

	return &Connection{
		logger:     logger,
		hostname:   hostName,
		username:   userName,
		repository: repository,
	}, nil
}

// Close closes the connection to the repository.
func (s *Connection) Close(ctx context.Context) error {
	err := s.repository.Close(ctx)
	if err != nil {
		return fmt.Errorf("while closing connection to Klio server: %w", err)
	}

	return nil
}

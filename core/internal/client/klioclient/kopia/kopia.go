package kopia

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"

	"github.com/kopia/kopia/repo"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	repository repo.Repository
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
	kopiaClientConfig *config.BaseRepositoryClientConfig,
) (*Connection, error) {
	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	certificateFingerprint, err := extractSHA256CertificateFingerprint(
		kopiaClientConfig.ServerCertPath)
	if err != nil {
		return nil, fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
	}

	clientCertificate, err := tls.LoadX509KeyPair(
		kopiaClientConfig.ClientCertPath,
		kopiaClientConfig.ClientKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("error while parsing client certificate: %w", err)
	}

	// Normalize the hostname by eventually removing
	// the '@<hostname>' suffix, which is already been
	// appended by Kopia.
	//
	// We should have a debate about
	// this. Should we really do it? Should we just
	// drop the suffix?
	hostName := kopiaClientConfig.Hostname
	userName := strings.TrimSuffix(clientCertificate.Leaf.Subject.CommonName, "@"+hostName)

	if err = repo.ConnectAPIServer(
		ctx,
		configFile.Name(),
		&repo.APIServerInfo{
			BaseURL:                             kopiaClientConfig.URL,
			TrustedServerCertificateFingerprint: certificateFingerprint,
			ClientCertificateFile:               kopiaClientConfig.ClientCertPath,
			ClientPrivateKeyFile:                kopiaClientConfig.ClientKeyPath,
		},
		"",
		&repo.ConnectOptions{
			ClientOptions: repo.ClientOptions{
				Hostname: hostName,
				Username: userName,
			},
		},
	); err != nil {
		return nil, fmt.Errorf("while pinging the repository: %w", err)
	}

	repository, err := repo.Open(ctx, configFile.Name(), "", &repo.Options{})
	if err != nil {
		return nil, fmt.Errorf("while opening the repository: %w", err)
	}

	return &Connection{
		repository: repository,
	}, nil
}

// GetUsername gets the username we are using for the connection.
// This is read from the client certificate.
func (s *Connection) GetUsername() string {
	return s.repository.ClientOptions().Username
}

// GetHostname gets the hostname we are using for the connection.
// This is read from the client certificate.
func (s *Connection) GetHostname() string {
	return s.repository.ClientOptions().Hostname
}

// Close closes the connection to the repository.
func (s *Connection) Close(ctx context.Context) error {
	err := s.repository.Close(ctx)
	if err != nil {
		return fmt.Errorf("while closing connection to Klio server: %w", err)
	}

	return nil
}

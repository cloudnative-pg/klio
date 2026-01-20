package kopia

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	kopiaBinary string
	hostName    string
	userName    string

	kopia *kopia.Client
}

// ConnectTier1 creates a new Kopia client to the tier1 kopia repository and opens a connection to it.
func ConnectTier1(
	ctx context.Context,
	kopiaClientConfig *config.BaseRepositoryClientConfig,
) (*Connection, error) {
	return internalConnect(ctx, kopiaClientConfig, kopiaClientConfig.URL)
}

// ConnectTier2 creates a new Kopia client and opens a connection to it.
func ConnectTier2(
	ctx context.Context,
	kopiaClientConfig *config.BaseRepositoryClientConfig,
) (*Connection, error) {
	return internalConnect(ctx, kopiaClientConfig, kopiaClientConfig.Tier2URL)
}

func internalConnect(
	ctx context.Context,
	kopiaClientConfig *config.BaseRepositoryClientConfig,
	kopiaURL string,
) (*Connection, error) {
	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	certificateFingerprint, err := kopia.ExtractSHA256CertificateFingerprint(
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

	if clientCertificate.Leaf == nil {
		return nil, fmt.Errorf("error: no leaf found in client certificate %+v", clientCertificate)
	}

	// Normalize the hostname by eventually removing
	// the '@<hostname>' suffix, which is already been
	// appended by Kopia.
	//
	// We should have a debate about
	// this. Should we really do it? Should we just
	// drop the suffix?
	userName, hostName, err := extractUserNameAndHostName(clientCertificate.Leaf.Subject.CommonName)
	if err != nil {
		return nil, fmt.Errorf("error while extracting userName and HostName from client certificate: %w", err)
	}

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	// This is a cache directory to speed up backup uploads.
	// We should have a debate about it. Should we really allocate it here?
	// Should we allocate a emptyDir volume just for it and use that?
	cacheDirectory := filepath.Join(os.TempDir(), "kopia-cache")

	if err := kopia.ConnectRemote(ctx, configFile.Name(), kopia.RemoteRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			PersistCredentials: false,
			CacheDirectory:     cacheDirectory,
		},
		URL:                   kopiaURL,
		ClientCertPath:        kopiaClientConfig.ClientCertPath,
		ClientKeyPath:         kopiaClientConfig.ClientKeyPath,
		ServerCertFingerprint: certificateFingerprint,
		Username:              userName,
		Hostname:              hostName,
	}); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	return &Connection{
		kopiaBinary: kopiaBinary,
		userName:    userName,
		hostName:    hostName,
		kopia: &kopia.Client{
			KopiaBinary: kopiaBinary,
			ConfigFile:  configFile.Name(),
			Password:    "mtls",
		},
	}, nil
}

// GetUsername gets the username we are using for the connection.
// This is read from the client certificate.
func (s *Connection) GetUsername() string {
	return s.userName
}

// GetHostname gets the hostname we are using for the connection.
// This is read from the client certificate.
func (s *Connection) GetHostname() string {
	return s.hostName
}

// Close closes the connection to the repository.
func (s *Connection) Close(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	if err := os.Remove(s.kopia.ConfigFile); err != nil {
		contextLogger.Error(
			err,
			"error while removing temporary Kopia configuration file, skipping",
			"configFile", s.kopia.ConfigFile)
	}

	return nil
}

// extractUserNameAndHostName from the common name in the client certificate.
// The common name must be in the form userName@hostName.
func extractUserNameAndHostName(commonName string) (string, string, error) {
	commonNameSplit := strings.Split(commonName, "@")
	if len(commonNameSplit) != 2 {
		return "", "", fmt.Errorf(`commonName must be in the form userName@hostName, got %q`, commonName)
	}

	if commonNameSplit[0] == "" {
		return "", "", fmt.Errorf(`userName part in commonName cannot be empty, got %q`, commonName)
	}

	if commonNameSplit[1] == "" {
		return "", "", fmt.Errorf(`hostName part in commonName cannot be empty, got %q`, commonName)
	}

	return commonNameSplit[0], commonNameSplit[1], nil
}

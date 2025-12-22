package kopia

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Connection represent a connection to a Klio server.
type Connection struct {
	configFile  string
	kopiaBinary string
	hostName    string
	userName    string
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
	contextLogger := log.FromContext(ctx)

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

	kopiaBinary, err := exec.LookPath(klioclient.KopiaCommand)
	if err != nil {
		return nil, fmt.Errorf("kopia binary not found (%q): %w", klioclient.KopiaCommand, err)
	}

	// This is a cache directory to speed up backup uploads.
	// We should have a debate about it. Should we really allocate it here?
	// Should we allocate a emptyDir volume just for it and use that?
	cacheDirectory := filepath.Join(os.TempDir(), "kopia-cache")

	args := []string{
		"repository",
		"connect",
		"server",
		"--disable-file-logging",
		"--cache-directory=" + cacheDirectory,
		"--json-log-console",
		"--config-file=" + configFile.Name(),
		"--url=" + kopiaURL,
		"--client-certificate=" + kopiaClientConfig.ClientCertPath,
		"--client-key=" + kopiaClientConfig.ClientKeyPath,
		"--server-cert-fingerprint=" + certificateFingerprint,
		"--override-username=" + userName,
		"--override-hostname=" + hostName,
	}

	repositoryConnectCmd := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	repositoryConnectCmd.Stdout = os.Stdout
	repositoryConnectCmd.Stderr = os.Stderr

	contextLogger.Info("Connecting to Kopia repository", "args", args)
	if err := repositoryConnectCmd.Run(); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	return &Connection{
		kopiaBinary: kopiaBinary,
		configFile:  configFile.Name(),
		userName:    userName,
		hostName:    hostName,
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

	if err := os.Remove(s.configFile); err != nil {
		contextLogger.Error(
			err,
			"error while removing temporary Kopia configuration file, skipping",
			"configFile", s.configFile)
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

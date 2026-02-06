package kopia

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// ErrClusterCertificateMismatch is returned when the configured cluster name
// does not match the hostname in the client certificate's Common Name.
var ErrClusterCertificateMismatch = errors.New("cluster name does not match certificate")

// Connection represent a connection to a Klio server.
type Connection struct {
	kopiaBinary string
	hostName    string
	userName    string

	kopia *kopia.Client
}

// CleanupConfigFile removes the Kopia configuration file and its associated password file.
// This function is idempotent and will not fail if the files are already gone.
func CleanupConfigFile(ctx context.Context, configFile string) {
	contextLogger := log.FromContext(ctx)

	removeIfExists := func(fileName string) {
		if err := os.Remove(fileName); err != nil {
			if !os.IsNotExist(err) {
				contextLogger.Error(
					err,
					"error while removing temporary Kopia configuration file, skipping",
					"configFile", fileName)
			}
		}
	}

	removeIfExists(configFile)
	removeIfExists(configFile + ".kopia-password")
}

// FromKopiaConfig creates a new Klio connection from an existing Kopia configuration file.
func FromKopiaConfig(configFile string) (*Connection, error) {
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return nil, err
	}

	configInfo, err := kopia.ParseConfigFile(configFile)
	if err != nil {
		return nil, err
	}

	return &Connection{
		kopiaBinary: kopiaBinary,
		hostName:    configInfo.HostName,
		userName:    configInfo.UserName,
		kopia: &kopia.Client{
			ConfigFile:  configFile,
			KopiaBinary: kopiaBinary,
		},
	}, nil
}

// ConnectTier1 creates a new Kopia client to the tier1 kopia repository and opens a connection to it.
func ConnectTier1(
	ctx context.Context,
	clientConfig *config.ClientConfig,
) (*Connection, error) {
	return connectToKopiaServer(ctx, clientConfig, clientConfig.Base.URL)
}

// ConnectTier2 creates a new Kopia client and opens a connection to it.
func ConnectTier2(
	ctx context.Context,
	clientConfig *config.ClientConfig,
) (*Connection, error) {
	return connectToKopiaServer(ctx, clientConfig, clientConfig.Base.Tier2URL)
}

func connectToKopiaServer(
	ctx context.Context,
	clientConfig *config.ClientConfig,
	kopiaURL string,
) (*Connection, error) {
	contextLogger := log.FromContext(ctx)

	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return nil, fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	certificateFingerprint, err := kopia.ExtractSHA256CertificateFingerprint(
		clientConfig.Base.ServerCertPath)
	if err != nil {
		return nil, fmt.Errorf("error while extracting fingerprint of the kopia server certificate: %w", err)
	}

	clientCertificate, err := tls.LoadX509KeyPair(
		clientConfig.Base.ClientCertPath,
		clientConfig.Base.ClientKeyPath,
	)
	if err != nil {
		return nil, fmt.Errorf("error while parsing client certificate: %w", err)
	}

	if clientCertificate.Leaf == nil {
		return nil, fmt.Errorf("error: no leaf found in client certificate %+v", clientCertificate)
	}

	// Extract username and hostname from certificate Common Name.
	// The CN must be in the format: userName@hostName
	userName, certHostName, err := extractUserNameAndHostName(clientCertificate.Leaf.Subject.CommonName)
	if err != nil {
		return nil, fmt.Errorf("error while extracting userName and hostName from client certificate: %w", err)
	}

	// Validate that the configured cluster name matches the certificate hostname.
	// This prevents silent failures where backups would be stored under a different
	// hostname and restore operations would fail to find them.
	if clientConfig.ClusterName != certHostName {
		contextLogger.Error(ErrClusterCertificateMismatch,
			"CONFIGURATION ERROR: cluster name and certificate mismatch",
			"configured_cluster_name", clientConfig.ClusterName,
			"certificate_hostname", certHostName,
			"certificate_CN", clientCertificate.Leaf.Subject.CommonName,
			"how_to_fix", fmt.Sprintf(
				"set 'cluster_name' to %q in your config, or use a certificate with CN '%s@%s'",
				certHostName, userName, clientConfig.ClusterName),
		)

		return nil, fmt.Errorf("%w: configured cluster %q but certificate has hostname %q",
			ErrClusterCertificateMismatch, clientConfig.ClusterName, certHostName)
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
		ClientCertPath:        clientConfig.Base.ClientCertPath,
		ClientKeyPath:         clientConfig.Base.ClientKeyPath,
		ServerCertFingerprint: certificateFingerprint,
		Username:              userName,
		Hostname:              certHostName,
	}); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	return &Connection{
		kopiaBinary: kopiaBinary,
		userName:    userName,
		hostName:    certHostName,
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
func (s *Connection) Close(ctx context.Context) {
	CleanupConfigFile(ctx, s.kopia.ConfigFile)
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

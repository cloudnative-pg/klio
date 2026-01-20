package kopia

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// ServerOptions contains the configuration options for starting a Kopia server.
type ServerOptions struct {
	// TLSCert is the path to the TLS certificate file for the server.
	TLSCert string

	// TLSKey is the path to the TLS private key file for the server.
	TLSKey string

	// ClientCACertFile is the path to the CA certificate file used to verify client certificates.
	ClientCACertFile string

	// ListenAddress is the address and port the server should listen on (e.g., "0.0.0.0:51515").
	ListenAddress string

	// ReadOnly indicates whether the server should operate in read-only mode.
	ReadOnly bool

	// ServerControlUser is the username for server control operations.
	ServerControlUser string

	// ServerControlPassword is the password for server control operations.
	ServerControlPassword string

	// EnableUI is true when the Kopia Javascript UI should be served by the Kopia server.
	EnableUI bool
}

// RunServer starts a Kopia server with the provided options.
func (s *Client) RunServer(ctx context.Context, opts ServerOptions) error {
	contextLogger := log.FromContext(ctx)

	// Start the Kopia server
	args := []string{
		"server", "start",
		"--tls-key-file=" + opts.TLSKey,
		"--tls-cert-file=" + opts.TLSCert,
		"--tls-ca-file=" + opts.ClientCACertFile,
		"--config-file=" + s.ConfigFile,
		"--address=" + opts.ListenAddress,
		"--disable-file-logging",
	}

	if opts.ReadOnly {
		args = append(args, "--readonly")
	}

	env := os.Environ()
	// Note: Kopia's 'server start' command uses KOPIA_SERVER_CONTROL_* variables
	if opts.ServerControlUser != "" {
		env = append(env, "KOPIA_SERVER_CONTROL_USER="+opts.ServerControlUser)
	}
	if opts.ServerControlPassword != "" {
		env = append(env, "KOPIA_SERVER_CONTROL_PASSWORD="+opts.ServerControlPassword)
	}
	if !opts.EnableUI {
		args = append(args, "--no-ui")
	}

	kopiaServer := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	kopiaServer.Env = env

	contextLogger.Info("Starting Kopia server", "args", kopiaServer.Args)

	if err := RunWithLogCapture(ctx, kopiaServer, nil); err != nil {
		return fmt.Errorf("while running the kopia server: %w", err)
	}

	return nil
}

// RefreshServerOptions contains the configuration options for refreshing a Kopia server.
type RefreshServerOptions struct {
	// ServerControlUser is the username for server control authentication.
	ServerControlUser string

	// ServerControlPassword is the password for server control authentication.
	ServerControlPassword string

	// ServerCertFingerprint is the SHA256 fingerprint of the server's certificate.
	ServerCertFingerprint string

	// Address is the address of the Kopia server to refresh.
	Address string
}

// RefreshServer triggers a repository refresh on a running Kopia server.
func (s *Client) RefreshServer(ctx context.Context, opts RefreshServerOptions) error {
	contextLogger := log.FromContext(ctx)

	// Trigger repository refresh on the Kopia server
	args := []string{
		"server", "refresh",
		"--address=" + opts.Address,
		"--disable-file-logging",
		"--server-cert-fingerprint=" + opts.ServerCertFingerprint,
	}

	env := os.Environ()
	// Note: Kopia's 'server refresh' command uses KOPIA_SERVER_USERNAME/PASSWORD
	// (different from 'server start' which uses KOPIA_SERVER_CONTROL_*)
	if opts.ServerControlUser != "" {
		env = append(env, "KOPIA_SERVER_USERNAME="+opts.ServerControlUser)
	}
	if opts.ServerControlPassword != "" {
		env = append(env, "KOPIA_SERVER_PASSWORD="+opts.ServerControlPassword)
	}

	kopiaServer := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	kopiaServer.Env = env

	contextLogger.Info("Refreshing kopia server", "args", kopiaServer.Args)

	if err := RunWithLogCapture(ctx, kopiaServer, nil); err != nil {
		return fmt.Errorf("while refreshing the kopia server: %w", err)
	}

	return nil
}

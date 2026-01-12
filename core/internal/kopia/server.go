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
		"--json-log-console",
	}

	if opts.ReadOnly {
		args = append(args, "--readonly")
	}

	kopiaServer := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	kopiaServer.Stdout = os.Stdout
	kopiaServer.Stderr = os.Stderr
	contextLogger.Info("Starting Kopia server", "args", kopiaServer.Args)

	if err := kopiaServer.Run(); err != nil {
		return fmt.Errorf("while running the kopia server: %w", err)
	}

	return nil
}

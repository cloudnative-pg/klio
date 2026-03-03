package kopia

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// RemoteRepoOpts contains options for network-based repositories.
type RemoteRepoOpts struct {
	CommonRepoOpts

	// URL is the remote server URL to connect to.
	URL string

	// ClientCertPath is the path to the client certificate file for TLS authentication.
	ClientCertPath string

	// ClientKeyPath is the path to the client private key file for TLS authentication.
	ClientKeyPath string

	// ServerCertFingerprint is the fingerprint of the server's TLS certificate for verification.
	ServerCertFingerprint string

	// Username is the username to use when connecting to the remote repository.
	Username string

	// Hostname is the hostname to use when connecting to the remote repository.
	Hostname string
}

// ConnectRemote connects to an existing Kopia repository via a remote server.
func ConnectRemote(ctx context.Context, configFileName string, opts RemoteRepoOpts) error {
	contextLogger := log.FromContext(ctx)

	args := buildConnectRemoteArgs(configFileName, opts)

	repositoryConnectCmd := exec.CommandContext(ctx, opts.KopiaBinary, args...) //nolint:gosec

	contextLogger.Info("Connecting to Kopia repository", "args", args)
	if err := RunWithLogCapture(ctx, repositoryConnectCmd, nil); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	return nil
}

// buildConnectRemoteArgs builds the argument list for the `kopia repository connect server` command.
func buildConnectRemoteArgs(configFileName string, opts RemoteRepoOpts) []string {
	args := []string{
		"repository",
		"connect",
		"server",
		"--disable-file-logging",
		"--cache-directory=" + opts.CacheDirectory,
		"--config-file=" + configFileName,
		"--url=" + opts.URL,
		"--client-certificate=" + opts.ClientCertPath,
		"--client-key=" + opts.ClientKeyPath,
		"--server-cert-fingerprint=" + opts.ServerCertFingerprint,
		"--override-username=" + opts.Username,
		"--override-hostname=" + opts.Hostname,
		"--metadata-cache-size-mb=0",
		"--content-cache-size-mb=0",
	}

	if opts.ReadOnly {
		args = append(args, "--readonly")
	}

	return args
}

/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package kopia

import (
	"context"
	"fmt"
	"os"
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
	repositoryConnectCmd.Env = getKopiaRepositoryConnectServerEnv()

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

// getKopiaRepositoryConnectServerEnv returns the environment variables
// for the `kopia repository connect server` command.
func getKopiaRepositoryConnectServerEnv() []string {
	env := os.Environ()
	env = append(env,
		"KOPIA_CHECK_FOR_UPDATES=false",
	)

	return env
}

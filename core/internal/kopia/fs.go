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
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// FSRepoOpts contains options for filesystem repository operations.
type FSRepoOpts struct {
	CommonRepoOpts

	// DataDirectory is the filesystem path where the repository data is stored.
	DataDirectory string
}

// InitializeFilesystem initializes a new Kopia repository on the filesystem.
func InitializeFilesystem(ctx context.Context, opts FSRepoOpts) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"repository", "create", "filesystem",
		"--create-only",
		"--path=" + opts.DataDirectory,
		"--disable-file-logging",
		"--cache-directory=" + opts.CacheDirectory,
	}

	kopiaRepositoryInitialize := exec.CommandContext(ctx, opts.KopiaBinary, args...) //nolint:gosec
	kopiaRepositoryInitialize.Env = append(kopiaRepositoryInitialize.Env,
		"KOPIA_PASSWORD="+opts.EncryptionPassword,
	)

	contextLogger.Info("Kopia repository initialize", "args", kopiaRepositoryInitialize.Args)
	if err := RunWithLogCapture(ctx, kopiaRepositoryInitialize, nil); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// ConnectFileSystem connects to an existing Kopia repository on the filesystem.
func ConnectFileSystem(ctx context.Context, configFileName string, opts FSRepoOpts) error {
	contextLogger := log.FromContext(ctx)

	args := buildConnectFSArgs(configFileName, opts)

	kopiaRepositoryConnect := exec.CommandContext(ctx, opts.KopiaBinary, args...) //nolint:gosec
	kopiaRepositoryConnect.Env = append(kopiaRepositoryConnect.Env,
		"KOPIA_PASSWORD="+opts.EncryptionPassword,
		"KOPIA_CHECK_FOR_UPDATES=false",
	)

	contextLogger.Info("Kopia repository connect", "args", kopiaRepositoryConnect.Args)
	if err := RunWithLogCapture(ctx, kopiaRepositoryConnect, nil); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

// buildConnectFSArgs builds the argument list for the `kopia repository connect filesystem` command.
func buildConnectFSArgs(configFileName string, opts FSRepoOpts) []string {
	args := []string{
		"repository", "connect", "filesystem",
		"--config-file=" + configFileName,
		"--path=" + opts.DataDirectory,
		"--override-username=klio",
		"--override-hostname=klio",
		"--disable-file-logging",
		"--cache-directory=" + opts.CacheDirectory,
	}

	if opts.PersistCredentials {
		args = append(args, "--persist-credentials")
	}

	if opts.ReadOnly {
		args = append(args, "--readonly")
	}

	return args
}

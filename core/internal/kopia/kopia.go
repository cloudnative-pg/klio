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
	"fmt"
	"os"
	"os/exec"
)

// Binary is name of the Kopia executable.
const Binary = "kopia"

// LookupBinary finds the Kopia binary in the system PATH and returns its absolute path.
// Returns an error if the binary is not found.
func LookupBinary() (string, error) {
	kopiaBinary, err := exec.LookPath(Binary)
	if err != nil {
		return "", fmt.Errorf("kopia binary not found (%q): %w", Binary, err)
	}

	return kopiaBinary, nil
}

// Client is a wrapper around the Kopia CLI.
type Client struct {
	// ConfigFile is the path to the Kopia configuration file.
	ConfigFile string

	// KopiaBinary is the path to the Kopia binary executable.
	KopiaBinary string

	// Password is the repository encryption password.
	Password string
}

func (s *Client) kopiaEnvironmentVariables() []string {
	result := os.Environ()
	result = append(result, "KOPIA_CHECK_FOR_UPDATES=false")
	if s.Password != "" {
		result = append(result, "KOPIA_PASSWORD="+s.Password)
	}
	result = append(result, tracingEnvironmentVariables()...)

	return result
}

// CommonRepoOpts contains common options for repository operations.
type CommonRepoOpts struct {
	// KopiaBinary is the path to the Kopia binary executable.
	KopiaBinary string

	// EncryptionPassword is the password used to encrypt the repository.
	EncryptionPassword string

	// PersistCredentials indicates whether credentials should be persisted in the config file.
	PersistCredentials bool

	// CacheDirectory is the directory used for caching repository data.
	CacheDirectory string

	// ReadOnly indicates whether the repository connection should be read-only.
	// When true, the --readonly flag is passed to `kopia repository connect`,
	// which is the correct way to enforce read-only access at the repository level.
	ReadOnly bool
}

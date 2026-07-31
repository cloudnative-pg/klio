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

package repository

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/afero"
)

// Connection represent a local connection to a Klio repository.
type Connection struct {
	config *KlioRepositoryConfig

	fs afero.Fs

	//nolint:godox
	// TODO(leonardoce)
	// Should we really keep the master key in our memory? Or should we seal it?
	// For now, let's start with this implementation and evaluate it later on.
	// The main points are:
	//
	// 1. for how much time this structure will be kept in memory?
	// 2. how much time is needed to decode the master key?
	masterKey []byte
}

// Open opens a connection to a repository.
func Open(options Options) (*Connection, error) {
	configFile, err := options.FS.OpenFile(repositoryConfigFileName, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("while opening configuration file %s: %w", repositoryConfigFileName, err)
	}
	defer func() {
		_ = configFile.Close()
	}()

	var config KlioRepositoryConfig
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		return nil, fmt.Errorf("while decoding JSON from configuration file %s: %w", repositoryConfigFileName, err)
	}

	masterKey, err := config.RecoverMasterKey(options.Password)
	if err != nil {
		return nil, fmt.Errorf("while recovering the master key: %w", err)
	}

	return &Connection{
		config:    &config,
		masterKey: masterKey,
		fs:        options.FS,
	}, nil
}

// Close closes the connection to the repository.
func (*Connection) Close() {}

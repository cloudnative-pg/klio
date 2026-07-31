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

package config

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/afero"
	"go.yaml.in/yaml/v3"
)

// DecodeYAML decodes YAML data into a Data struct using mapstructure.
// This ensures consistent behavior with Viper by respecting mapstructure tags.
func DecodeYAML(r io.Reader) (*Data, error) {
	var rawConfig map[string]any
	if err := yaml.NewDecoder(r).Decode(&rawConfig); err != nil {
		return nil, fmt.Errorf("decoding YAML: %w", err)
	}

	var data Data
	if err := mapstructure.Decode(rawConfig, &data); err != nil {
		return nil, fmt.Errorf("mapping configuration: %w", err)
	}

	return &data, nil
}

// NewFromFile loads the configuration from a YAML file at the given path.
func NewFromFile(fs afero.Fs, path string) (*Data, error) {
	configFile, err := fs.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("while loading config file %q: %w", path, err)
	}
	defer func() {
		_ = configFile.Close()
	}()

	configuration, err := DecodeYAML(configFile)
	if err != nil {
		return nil, fmt.Errorf("while decoding config file %q: %w", path, err)
	}

	return configuration, nil
}

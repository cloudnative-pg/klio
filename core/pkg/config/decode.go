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

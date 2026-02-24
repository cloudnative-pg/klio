package config

import (
	"fmt"
	"io"

	"github.com/go-viper/mapstructure/v2"
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

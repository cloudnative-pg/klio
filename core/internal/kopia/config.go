package kopia

import (
	"encoding/json"
	"fmt"
	"os"
)

// ConfigData represents the parsed data from a Kopia configuration file.
type ConfigData struct {
	HostName    string `json:"hostname"`
	UserName    string `json:"username"`
	Description string `json:"description"`
}

// ParseConfigFile reads and parses a Kopia configuration file.
func ParseConfigFile(configFile string) (*ConfigData, error) {
	// #nosec G304 -- configFile path is from internal config
	f, err := os.Open(configFile)
	if err != nil {
		return nil, fmt.Errorf("while opening config file %q: %w", configFile, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var result ConfigData
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		return nil, fmt.Errorf("while reading from config file %q: %w", configFile, err)
	}

	return &result, nil
}

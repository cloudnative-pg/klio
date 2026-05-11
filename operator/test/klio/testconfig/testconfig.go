// Package testconfig loads e2e test configuration from a YAML file.
//
// Configuration is resolved in two layers, each overriding the previous:
//  1. Built-in defaults (always applied first)
//  2. YAML config file (default: e2e-config.yaml, override via E2E_CONFIG_FILE)
package testconfig

import (
	"errors"
	"os"

	"github.com/spf13/viper"
)

// Config value defaults.
const (
	defaultServerImage = "registry.dev:5000/klio-testing:dev"
	defaultLogDir      = "e2e_cluster_logs"
)

// Config file loader settings.
const (
	defaultConfigFile = "e2e-config.yaml"
	envConfigFile     = "E2E_CONFIG_FILE"
)

// ImagePullSecretConfig holds registry credentials used to create a
// kubernetes.io/dockerconfigjson pull secret in each test namespace.
type ImagePullSecretConfig struct {
	// Registry is the registry hostname (e.g. "ghcr.io").
	Registry string `mapstructure:"registry"`
	// Username is the registry username.
	Username string `mapstructure:"username"`
	// Password is the registry password or token.
	Password string `mapstructure:"password"`
}

// IsConfigured returns true when all three credential fields are non-empty.
func (c ImagePullSecretConfig) IsConfigured() bool {
	return c.Registry != "" && c.Username != "" && c.Password != ""
}

// Config holds the e2e test configuration.
type Config struct {
	// ServerImage is the Klio server container image used in tests.
	ServerImage string `mapstructure:"serverImage"`

	// LogDir is the directory where pod logs are streamed during the test run.
	LogDir string `mapstructure:"logDir"`

	// StorageClass is the Kubernetes storage class used for all PVC templates
	// in the tests (tier1 cache, tier1 data, queue, tier2 cache).
	StorageClass string `mapstructure:"storageClass"`

	// ImagePullSecret holds optional registry credentials. When all fields are
	// non-empty, a pull secret named "e2e-pull-secret" is created in every test
	// namespace and referenced by the Server and Cluster templates.
	ImagePullSecret ImagePullSecretConfig `mapstructure:"imagePullSecret"`
}

// Load reads the e2e test configuration. It applies built-in defaults first,
// then overlays values from the YAML config file.
// If the config file does not exist, built-in defaults are used without error.
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("serverImage", defaultServerImage)
	v.SetDefault("logDir", defaultLogDir)

	path := os.Getenv(envConfigFile)
	if path == "" {
		path = defaultConfigFile
	}
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

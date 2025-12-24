package config

import (
	"errors"

	"gopkg.in/validator.v2"
)

// ErrMissingTier2Configuration is raised when no tier2 backend
// has been configured.
var ErrMissingTier2Configuration = errors.New(
	"tier2 is disabled, s3 configuration is missing")

// RequireTier1 checks if the configuration settings
// for tier1 are correctly set.
func (c *ServerConfig) RequireTier1() error {
	var result error

	if err := validator.Validate(&c.TLS); err != nil {
		result = errors.Join(result, err)
	}

	if err := validator.Validate(&c.Tier1); err != nil {
		result = errors.Join(result, err)
	}

	return result
}

// RequireTier2 checks if the configuration settings
// for tier1 are correctly set.
func (c *ServerConfig) RequireTier2() error {
	var result error

	if err := validator.Validate(&c.TLS); err != nil {
		result = errors.Join(result, err)
	}

	if err := validator.Validate(&c.Tier2); err != nil {
		result = errors.Join(result, err)
	}

	if !c.Tier2.S3.Enabled {
		return ErrMissingTier2Configuration
	}

	return result
}

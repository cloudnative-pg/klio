package config

import (
	"errors"
)

// ErrMissingTier2Configuration is raised when no tier2 backend
// has been configured.
var ErrMissingTier2Configuration = errors.New(
	"tier2 is disabled, s3 configuration is missing")

// Validate implements a custom validation function for ClientConfig.
func (c *ServerConfig) Validate() error {
	var errs error

	if err := c.TLS.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.Tier1.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.Tier2.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// Validate implements a custom validation function for TLSConfig.
func (c *TLSConfig) Validate() error {
	var errs error

	if c.TLSCert == "" {
		errs = errors.Join(errs, errors.New("invalid TLS config: tls_cert is empty"))
	}
	if c.TLSKey == "" {
		errs = errors.Join(errs, errors.New("invalid TLS config: tls_key is empty"))
	}
	if c.ClientCACertFile == "" {
		errs = errors.Join(errs, errors.New("invalid TLS config: client_ca_cert is empty"))
	}

	return errs
}

// Validate implements a custom validation function for Tier1Config.
func (c *Tier1Config) Validate() error {
	var errs error

	if c.EncryptionKey == "" {
		errs = errors.Join(errs, errors.New("invalid tier1 config: encryption_key is empty"))
	}
	if err := c.Base.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.Wal.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// Validate implements a custom validation function for Tier2Config.
func (c *Tier2Config) Validate() error {
	var errs error

	if c.S3.Enabled {
		if c.EncryptionKey == "" {
			errs = errors.Join(errs, errors.New("invalid tier2 config: encryption_key is empty"))
		}
		if c.BaseListenAddress == "" {
			errs = errors.Join(errs, errors.New("invalid tier2 config: base_listen_address is empty"))
		}
		if c.WALListenAddress == "" {
			errs = errors.Join(errs, errors.New("invalid tier2 config: wal_listen_address is empty"))
		}
		if c.CacheDirectory == "" {
			errs = errors.Join(errs, errors.New("invalid tier2 config: cache is empty"))
		}
	}

	// The only validation we do is on the S3 configuration, as
	// the other parameters may be empty
	if err := c.S3.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// Validate implements a custom validation function for BaseServerConfig.
func (c *BaseServerConfig) Validate() error {
	var errs error

	if c.CacheDirectory == "" {
		errs = errors.Join(errs, errors.New("invalid base server config: cache is empty"))
	}
	if c.RepositoryDirectory == "" {
		errs = errors.Join(errs, errors.New("invalid base server config: repository is empty"))
	}
	if c.ListenAddress == "" {
		errs = errors.Join(errs, errors.New("invalid base server config: listen_address is empty"))
	}

	return errs
}

// Validate implements a custom validation function for WalServerConfig.
func (c *WalServerConfig) Validate() error {
	var errs error

	if c.ListenAddress == "" {
		errs = errors.Join(errs, errors.New("invalid wal server config: listen_address is empty"))
	}
	if c.WALPath == "" {
		errs = errors.Join(errs, errors.New("invalid wal server config: path is empty"))
	}

	return errs
}

// Validate implements a custom validation function for S3Configuration.
func (c *S3Configuration) Validate() error {
	var errs error

	if !c.Enabled {
		return nil
	}
	if c.BucketName == "" {
		errs = errors.Join(errs, errors.New("invalid s3 config: bucket_name is empty"))
	}
	if c.Endpoint == "" {
		errs = errors.Join(errs, errors.New("invalid s3 config: endpoint is empty"))
	}
	if c.Region == "" {
		errs = errors.Join(errs, errors.New("invalid s3 config: region is empty"))
	}

	return errs
}

// RequireTier1 checks if the configuration settings
// for tier1 are correctly set.
func (c *ServerConfig) RequireTier1() error {
	var errs error

	if err := c.TLS.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	if err := c.Tier1.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// RequireTier2 checks if the configuration settings
// for tier2 are correctly set.
func (c *ServerConfig) RequireTier2() error {
	var errs error

	if err := c.TLS.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	if err := c.Tier2.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	if !c.Tier2.S3.Enabled {
		errs = errors.Join(errs, ErrMissingTier2Configuration)
	}

	return errs
}

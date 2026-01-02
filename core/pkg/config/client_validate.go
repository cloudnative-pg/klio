package config

import (
	"errors"
	"regexp"
)

// Validate implements a custom validation function for Data.
func (d *Data) Validate() error {
	var errs error

	if (d.Source != SourceConfig{}) {
		if err := d.Source.Validate(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if err := d.Client.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}

	return errs
}

// Validate implements a custom validation function for SourceConfig.
func (s *SourceConfig) Validate() error {
	var errs error

	if s.DSN == "" {
		errs = errors.Join(errs, errors.New("invalid source config: dsn is empty"))
	}
	if s.StandardDSN == "" {
		errs = errors.Join(errs, errors.New("invalid source config: standard_dsn is empty"))
	}
	if s.Slot == "" {
		errs = errors.Join(errs, errors.New("invalid source config: slot is empty"))
	}
	if matched, _ := regexp.MatchString("^[a-z0-9_]+$", s.Slot); !matched {
		errs = errors.Join(errs, errors.New(
			"invalid source config: slot name can only contain lower-case letters, numbers, and underscores"))
	}
	if s.StandbyMessageTimeoutSeconds < 1 {
		errs = errors.Join(errs, errors.New("invalid source config: standby_message_timeout_seconds must be at least 1"))
	}
	if s.FlushTimeoutMilliseconds < 1 {
		errs = errors.Join(errs, errors.New("invalid source config: flush_timeout_ms must be at least 1"))
	}
	if s.BufferSize < 1 {
		errs = errors.Join(errs, errors.New("invalid source config: buffer_size must be at least 1"))
	}

	return errs
}

// Validate implements a custom validation function for ClientConfig.
func (c *ClientConfig) Validate() error {
	var errs error

	if c.Base != (BaseRepositoryClientConfig{}) {
		if err := c.Base.Validate(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if c.Wal != (WalRepositoryClientConfig{}) {
		if err := c.Wal.Validate(); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

// Validate implements a custom validation function for WalRepositoryClientConfig.
func (c *WalRepositoryClientConfig) Validate() error {
	var errs error

	if c.Address == "" && c.Tier2Address == "" {
		errs = errors.Join(errs, errors.New(
			"invalid wal repository client config: at least one of address or tier2_address must be specified"))
	}

	if c.ClusterName == "" {
		errs = errors.Join(errs, errors.New("invalid wal repository client config: cluster_name is empty"))
	}
	if c.ServerCertPath == "" {
		errs = errors.Join(errs, errors.New("invalid wal repository client config: server_cert_path is empty"))
	}
	if c.ClientCertPath == "" {
		errs = errors.Join(errs, errors.New("invalid wal repository client config: client_cert_path is empty"))
	}
	if c.ClientKeyPath == "" {
		errs = errors.Join(errs, errors.New("invalid wal repository client config: client_key_path is empty"))
	}

	return errs
}

// Validate implements a custom validation function for BaseRepositoryClientConfig.
func (c *BaseRepositoryClientConfig) Validate() error {
	var errs error

	if c.URL == "" && c.Tier2URL == "" {
		errs = errors.Join(errs, errors.New(
			"invalid base repository client config: at least one of url or tier2_url must be specified"))
	}

	if c.ServerCertPath == "" {
		errs = errors.Join(errs, errors.New("invalid base repository client config: server_cert_path is empty"))
	}
	if c.ClientCertPath == "" {
		errs = errors.Join(errs, errors.New("invalid base repository client config: client_cert_path is empty"))
	}
	if c.ClientKeyPath == "" {
		errs = errors.Join(errs, errors.New("invalid base repository client config: client_key_path is empty"))
	}

	return errs
}

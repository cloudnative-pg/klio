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
	"errors"
	"fmt"
	"path/filepath"
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

	if c.EncryptionKeyFile == "" {
		errs = errors.Join(errs, errors.New("invalid tier1 config: encryption_key_file is empty"))
	}
	if err := c.Base.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}
	if err := c.Wal.Validate(); err != nil {
		errs = errors.Join(errs, err)
	}
	if c.Compression.Algorithm != "" && !IsValidCompressionAlgorithm(c.Compression.Algorithm) {
		errs = errors.Join(errs, fmt.Errorf("invalid tier1 config: %w: %q",
			ErrInvalidCompressionAlgorithm, c.Compression.Algorithm))
	}

	return errs
}

// Validate implements a custom validation function for Tier2Config.
func (c *Tier2Config) Validate() error {
	var errs error

	if c.S3.Enabled {
		if c.EncryptionKeyFile == "" {
			errs = errors.Join(errs, errors.New("invalid tier2 config: encryption_key_file is empty"))
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

	if c.Compression.Algorithm != "" && !IsValidCompressionAlgorithm(c.Compression.Algorithm) {
		errs = errors.Join(errs, fmt.Errorf("invalid tier2 config: %w: %q",
			ErrInvalidCompressionAlgorithm, c.Compression.Algorithm))
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
	if !c.Enabled {
		return nil
	}

	var errs error
	if c.BucketName == "" {
		errs = errors.Join(errs, errors.New("invalid s3 config: bucket_name is empty"))
	}
	if c.Region == "" {
		errs = errors.Join(errs, errors.New("invalid s3 config: region is empty"))
	}

	errs = errors.Join(errs, c.validateAWSCredentials())

	return errs
}

// validateAWSCredentials validates AWS authentication methods.
// Credentials are optional; when not provided, the AWS SDK will automatically
// use IAM role credentials (EKS IRSA, or Pod Identity).
// If credentials are partially provided, an error is returned.
func (c *S3Configuration) validateAWSCredentials() error {
	// If both access key ID and secret are provided, that's valid
	if c.AccessKeyID != "" && c.SecretAccessKey != "" {
		return nil
	}

	// If neither is provided, that's valid (will use IAM role)
	if c.AccessKeyID == "" && c.SecretAccessKey == "" {
		return nil
	}

	// If only one is provided, that's an error
	return errors.New(
		"invalid s3 config: when using AWS credentials both access_key_id and secret_access_key must be provided")
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

	// Resolve absolute paths to catch collisions that use different naming conventions
	cleanDir1, err := filepath.Abs(filepath.Clean(c.Tier1.Base.CacheDirectory))
	if err != nil {
		errs = errors.Join(errs, err)
	}
	cleanDir2, err := filepath.Abs(filepath.Clean(c.Tier2.CacheDirectory))
	if err != nil {
		errs = errors.Join(errs, err)
	}

	// Validate that tier1 and tier2 don't use the same cache directory
	if cleanDir1 == cleanDir2 {
		errs = errors.Join(errs, errors.New(
			"tier1 and tier2 cannot use the same cache directory"))
	}

	return errs
}

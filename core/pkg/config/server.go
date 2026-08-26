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

// ServerConfig is the configuration of the Klio server.
type ServerConfig struct {
	// TLS is the TLS configuration of the base and of the WAL
	// server
	TLS TLSConfig `mapstructure:"tls"`

	// Tier1Config is the Tier 1 configuration
	Tier1 Tier1Config `mapstructure:"tier1"`

	// Tier2Config is the Tier 2 configuration
	Tier2 Tier2Config `mapstructure:"tier2"`

	// QueueDirectory is the directory where the persistent queue
	// messages will be stored.
	QueueDirectory string `mapstructure:"queue_directory"`
}

// TLSConfig is the TLS configuration of the server.
type TLSConfig struct {
	// TLSCert is the path to the server public key
	TLSCert string `json:"tls_cert" mapstructure:"cert"`

	// TLSKey is the path to the server private key
	TLSKey string `json:"tls_key" mapstructure:"key"`

	// ClientCACertFile is the file containing the CA certificate to be used
	// to verify client certificates
	ClientCACertFile string `json:"client_ca_cert" mapstructure:"client_ca_cert"`
}

// Tier1Config is the configuration of tier 1.
type Tier1Config struct {
	// EncryptionKeyFile is the path to the file containing the encryption
	// key used to operate on the Kopia repository and on the WAL directory.
	// When IdentityFile is set, this file is Age-encrypted and will be
	// decrypted at startup. Otherwise it is read as plaintext.
	EncryptionKeyFile string `mapstructure:"encryption_key_file"`

	// IdentityFile is the path to the Age identity file used to decrypt
	// the encryption key file. When empty, EncryptionKeyFile is read as
	// plaintext.
	IdentityFile string `mapstructure:"identity_file"`

	// EncryptionKey is the encryption key loaded from EncryptionKeyFile at startup.
	// Populated by LoadEncryptionKey; not read from configuration.
	EncryptionKey string `mapstructure:"-"`

	// Base is the configuration of the Base server
	Base BaseServerConfig `mapstructure:"base"`

	// Wal is the configuration of the Wal server
	Wal WalServerConfig `mapstructure:"wal"`

	// Compression is the repository-wide (global) compression policy applied
	// to base backups stored on tier1. When empty, the Kopia default (no
	// compression) is left untouched.
	Compression CompressionServerConfig `mapstructure:"compression"`
}

// Tier2Config is the configuration of tier 2.
type Tier2Config struct {
	// EncryptionKeyFile is the path to the file containing the encryption key.
	// When IdentityFile is set, this file is Age-encrypted and will be
	// decrypted at startup. Otherwise it is read as plaintext.
	EncryptionKeyFile string `mapstructure:"encryption_key_file"`

	// IdentityFile is the path to the Age identity file used to decrypt
	// the encryption key file. When empty, EncryptionKeyFile is read as
	// plaintext.
	IdentityFile string `mapstructure:"identity_file"`

	// EncryptionKey is the encryption key loaded from EncryptionKeyFile at startup.
	// Populated by LoadEncryptionKey; not read from configuration.
	EncryptionKey string `mapstructure:"-"`

	// BaseListenAddress is the address where the tier2 base server will listen
	BaseListenAddress string `mapstructure:"base_listen_address"`

	// WALListenAddress is the address where the tier2 wal server will listen
	WALListenAddress string `mapstructure:"wal_listen_address"`

	// CacheDirectory is the directory of the Kopia cache
	CacheDirectory string `mapstructure:"cache"`

	// Compression is the repository-wide (global) compression policy applied
	// to base backups stored on tier2. When empty, the Kopia default (no
	// compression) is left untouched.
	Compression CompressionServerConfig `mapstructure:"compression"`

	// S3 contains the configuration parameters for an S3-based tier 2
	S3 S3Configuration `json:"s3" mapstructure:"s3"`
}

// CompressionServerConfig is the repository-wide (global) compression policy
// applied to a tier when the Kopia server starts.
type CompressionServerConfig struct {
	// Algorithm is the name of the Kopia compression algorithm to use.
	// The special value "none" disables compression.
	Algorithm string `mapstructure:"algorithm"`

	// MinSize is the minimum file size, in bytes, to attempt compression for.
	// Zero means no minimum.
	MinSize int64 `mapstructure:"min_size"`

	// MaxSize is the maximum file size, in bytes, to attempt compression for.
	// Zero means no maximum.
	MaxSize int64 `mapstructure:"max_size"`
}

// IsZero reports whether the compression policy carries no settings.
func (c CompressionServerConfig) IsZero() bool {
	return c.Algorithm == "" && c.MinSize == 0 && c.MaxSize == 0
}

// BaseServerConfig is the configuration that will be used for
// the kopia server.
type BaseServerConfig struct {
	// CacheDirectory is the directory of the Kopia cache
	CacheDirectory string `mapstructure:"cache"`

	// RepositoryDirectory is the directory where the Kopia repository is stored.
	RepositoryDirectory string `mapstructure:"repository"`

	// ListenAddress is the address where we should listen to
	ListenAddress string `mapstructure:"listen_address"`
}

// WalServerConfig is the configuration of the Klio server.
type WalServerConfig struct {
	// ListenAddress is the listening address
	ListenAddress string `json:"listen_address" mapstructure:"listen_address"`

	// WALPath is the path where the WALs should be stored
	WALPath string `json:"path" mapstructure:"path"`
}

// S3Configuration is the configuration to a S3 defined tier 2.
type S3Configuration struct {
	// Enabled is true when S3 is enabled
	Enabled bool `json:"enabled" mapstructure:"enabled"`

	// BucketName is the name of the bucket
	BucketName string `json:"bucket_name" mapstructure:"bucket_name"`

	// Endpoint is the AWS endpoint to be used
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`

	// Region is the AWS region to be used.
	Region string `json:"region" mapstructure:"region"`

	// Prefix is the path within the bucket under which all Klio objects
	// are stored, allowing a single bucket to be shared across multiple deployments.
	Prefix string `json:"prefix" mapstructure:"prefix"`

	// AccessKeyID is the access key ID
	AccessKeyID string `json:"access_key_id" mapstructure:"access_key_id"`

	// SecretAccessKey is the secret access key
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key"`

	// SessionToken is the session token
	SessionToken string `json:"session_token" mapstructure:"session_token"`

	// CustomCABundleFile is the file where we should read the custom CA bundle
	CustomCABundleFile string `json:"custom_ca_bundle_file" mapstructure:"custom_ca_bundle_file"`
}

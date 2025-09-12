// Package config contains the configuration data structure and the relative
// helpers
package config

import "time"

// Data is the configuration.
//
// This struct is used to generate a secret in the Kubernetes cluster, so its serialization must be stable.
type Data struct {
	// Source is the configuration of the database we should collect WALs for.
	// This is only needed fot the WAL pusher.
	Source SourceConfig `json:"source" mapstructure:"source"`

	// Client is the configuration of the Klio client
	Client ClientConfig `json:"client" mapstructure:"client"`
}

// SetDefaults sets the default values of the configuration.
func (d *Data) SetDefaults() {
	if d.Source != (SourceConfig{}) {
		d.Source.SetDefaults()
	}
}

// ServerConfig is the configuration of the Klio server.
type ServerConfig struct {
	// Base is the configuration of the Base server
	Base BaseServerConfig `mapstructure:"base" validate:"nonzero"`

	// Wal is the configuration of the Wal server
	Wal WalServerConfig `mapstructure:"wal" validate:"nonzero"`
}

// BaseServerConfig is the configuration that will be used for
// the kopia server.
type BaseServerConfig struct {
	// EncryptionPassword is the encryption password that is used to create and
	// operate on the Kopia repository.
	EncryptionPassword string `mapstructure:"encryption_password" validate:"nonzero"`

	// CacheDirectory is the directory of the Kopia cache
	CacheDirectory string `mapstructure:"cache" validate:"nonzero"`

	// RepositoryDirectory is the directory where the Kopia repository is stored.
	RepositoryDirectory string `mapstructure:"repository" validate:"nonzero"`

	// TLSKey is the file of the TLS private key
	TLSKey string `mapstructure:"tls_key" validate:"nonzero"`

	// TLSCert is the file of the TLS public key
	TLSCert string `mapstructure:"tls_cert" validate:"nonzero"`

	// ListenAddress is the address where we should listen to
	ListenAddress string `mapstructure:"listen_address" validate:"nonzero"`

	// HTPasswdFile is the file containing the credentials of the users that are
	// allowed to use the Kopia server
	HTPasswdFile string `mapstructure:"htpasswd_file" validate:"nonzero"`

	// AdminUser kopia super-user name
	AdminUser string `mapstructure:"admin_user"`

	// AdminPassword kopia super-user password
	AdminPassword string `mapstructure:"admin_password"`
}

// SourceConfig is the configuration of the WAL receiver.
type SourceConfig struct {
	// DSN is the database service we should get the WALs from
	DSN string `json:"dsn" mapstructure:"dsn" validate:"nonzero"`

	// StandardDSN is the database service name to be used for a standard
	// database connection
	StandardDSN string `json:"standard_dsn" mapstructure:"standard_dsn" validate:"nonzero"`

	// Slot is the name of the replication slot to be used
	Slot string `json:"slot" mapstructure:"slot" validate:"nonzero,regexp=^[a-z0-9_]+$"`

	// StandbyMessageTimeoutSeconds is the timeout after which the WAL
	// receiver will send a status update
	StandbyMessageTimeoutSeconds int `json:"standby_message_timeout_seconds" mapstructure:"standby_message_timeout_seconds" validate:"min=1"` //nolint:lll

	// FlushTimeoutMilliseconds is the timeout in milliseconds after which buffered
	// WAL data is automatically flushed to the Klio server
	FlushTimeoutMilliseconds int `json:"flush_timeout_ms" mapstructure:"flush_timeout_ms" validate:"min=1"`

	// BufferSize is the maximum size in bytes of the in-memory WAL buffer before
	// triggering an automatic flush
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size" validate:"min=1"`
}

// ClientConfig is the configuration of the Klio client.
type ClientConfig struct {
	// Base is the configuration of the target Base repository
	Base BaseRepositoryClientConfig `json:"base" mapstructure:"base"`

	// Wal is the configuration of the Wal repository
	Wal WalRepositoryClientConfig `json:"wal" mapstructure:"wal"`
}

// WalRepositoryClientConfig is the configuration of the Klio repository
// where WALs should be uploaded.
type WalRepositoryClientConfig struct {
	// Address of the Klio server
	Address string `json:"address" mapstructure:"address" validate:"nonzero"`

	// ClusterName is the name of the target cluster where to upload WALs
	ClusterName string `json:"cluster_name" mapstructure:"cluster_name" validate:"nonzero"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `json:"server_cert_path" mapstructure:"server_cert_path" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `json:"username" mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `json:"password" mapstructure:"password" validate:"nonzero"`
}

// BaseRepositoryClientConfig is the configuration of the Kopia repository
// to be used to upload the data directory.
type BaseRepositoryClientConfig struct {
	// URL is the base URL where the Kopia API server should be reached
	URL string `json:"url" mapstructure:"url" validate:"nonzero"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `json:"server_cert_path" mapstructure:"server_cert_path" validate:"nonzero"`

	// Hostname is the Klio server hostname.
	// This is used to create the full username, in the form <username>@<hostname>
	Hostname string `json:"hostname" mapstructure:"hostname" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `json:"username" mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `json:"password" mapstructure:"password" validate:"nonzero"`
}

// WalServerConfig is the configuration of the Klio server.
type WalServerConfig struct {
	// ListenAddress is the listening address
	ListenAddress string `json:"listen_address" mapstructure:"listen_address" validate:"nonzero"`

	// TLSCert is the path to the server public key
	TLSCert string `json:"tls_cert" mapstructure:"tls_cert" validate:"nonzero"`

	// TLSKey is the path to the server private key
	TLSKey string `json:"tls_key" mapstructure:"tls_key" validate:"nonzero"`

	// WALPath is the path where the WALs should be stored
	WALPath string `json:"path" mapstructure:"path" validate:"nonzero"`

	// EncryptionPassword is the encryption password
	EncryptionPassword string `json:"encryption_password" mapstructure:"encryption_password" validate:"nonzero"`

	// HTPasswdFile is the file containing the credentials of the users that are
	// allowed to use the Kopia server
	HTPasswdFile string `json:"htpasswd_file" mapstructure:"htpasswd_file" validate:"nonzero"`
}

// SetDefaults sets the default values of the configuration.
func (s *SourceConfig) SetDefaults() {
	s.StandbyMessageTimeoutSeconds = 10
	s.FlushTimeoutMilliseconds = 200
	s.BufferSize = 2 * 1024 * 1024 // 2 MB
}

// StandbyMessageTimeout returns the stanby message timeout in a
// time.Duration.
func (s *SourceConfig) StandbyMessageTimeout() time.Duration {
	return time.Second * time.Duration(s.StandbyMessageTimeoutSeconds)
}

// FlushTimeout returns the timeout after which the WALs are
// flushed.
func (s *SourceConfig) FlushTimeout() time.Duration {
	return time.Millisecond * time.Duration(s.FlushTimeoutMilliseconds)
}

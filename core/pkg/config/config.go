// Package config contains the configuration data structure and the relative
// helpers
package config

import "time"

// Data is the configuration.
type Data struct {
	// Source is the configuration of the database we should collect WALs for.
	// This is only needed fot the WAL pusher.
	Source SourceConfig `mapstructure:"source"`

	// Client is the configuration of the Klio client
	Client ClientConfig `mapstructure:"client"`
}

// SetDefaults sets the default values of the configuration.
func (d *Data) SetDefaults() {
	if d.Source != (SourceConfig{}) {
		d.Source.SetDefaults()
	}
}

// ServerConfig is the configuration of the server.
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
	DSN string `mapstructure:"dsn" validate:"nonzero"`

	// StandardDSN is the database service name to be used for a standard
	// database connection
	StandardDSN string `mapstructure:"standard_dsn" validate:"nonzero"`

	// Slot is the name of the replication slot to be used
	Slot string `mapstructure:"slot" validate:"nonzero,regexp=^[a-z0-9_]+$"`

	// StandbyMessageTimeoutSeconds is the timeout after which the WAL
	// receiver will send a status update
	StandbyMessageTimeoutSeconds int `mapstructure:"standby_message_timeout_seconds" validate:"min=1"`
}

// ClientConfig is the configuration of the Klio server.
type ClientConfig struct {
	// Base is the configuration of the target Base repository
	Base BaseRepositoryClientConfig `mapstructure:"base"`

	// Wal is the configuration of the Wal repository
	Wal WalRepositoryClientConfig `mapstructure:"wal"`
}

// WalRepositoryClientConfig is the configuration of the Klio repository
// where WALs should be uploaded.
type WalRepositoryClientConfig struct {
	// Address of the Klio server
	Address string `validate:"nonzero"`

	// ClusterName is the name of the target cluster where to upload WALs
	ClusterName string `mapstructure:"cluster_name" validate:"nonzero"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `mapstructure:"server_cert_path" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `mapstructure:"password" validate:"nonzero"`
}

// BaseRepositoryClientConfig is the configuration of the Kopia repository
// to be used to upload the data directory.
type BaseRepositoryClientConfig struct {
	// URL is the base URL where the Kopia API server should be reached
	URL string `mapstructure:"url" validate:"nonzero"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `mapstructure:"server_cert_path" validate:"nonzero"`

	// Hostname is the Klio server hostname.
	// This is used to create the full username, in the form <username>@<hostname>
	Hostname string `mapstructure:"hostname" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `mapstructure:"password" validate:"nonzero"`
}

// WalServerConfig is the configuration of the Klio server.
type WalServerConfig struct {
	// ListenAddress is the listening address
	ListenAddress string `mapstructure:"listen_address" validate:"nonzero"`

	// TLSCert is the path to the server public key
	TLSCert string `mapstructure:"tls_cert" validate:"nonzero"`

	// TLSKey is the path to the server private key
	TLSKey string `mapstructure:"tls_key" validate:"nonzero"`

	// WALPath is the path where the WALs should be stored
	WALPath string `mapstructure:"path" validate:"nonzero"`

	// EncryptionPassword is the encryption password
	EncryptionPassword string `mapstructure:"encryption_password" validate:"nonzero"`

	// HTPasswdFile is the file containing the credentials of the users that are
	// allowed to use the Kopia server
	HTPasswdFile string `mapstructure:"htpasswd_file" validate:"nonzero"`
}

// SetDefaults sets the default values of the configuration.
func (s *SourceConfig) SetDefaults() {
	s.StandbyMessageTimeoutSeconds = 10
}

// StandbyMessageTimeout returns the stanby message timeout in a
// time.Duration.
func (s *SourceConfig) StandbyMessageTimeout() time.Duration {
	return time.Second * time.Duration(s.StandbyMessageTimeoutSeconds)
}

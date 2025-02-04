// Package config contains the configuration data structure and the relative
// helpers
package config

import "time"

// Data is the configuration.
type Data struct {
	// ClusterName is the name of the cluster
	ClusterName string `mapstructure:"cluster_name" validate:"nonzero"`

	// Source is the configuration of the database we should collect WALs for.
	// This is only needed fot the WAL pusher.
	Source *Source `mapstructure:"source"`

	// Client is the configuration of the Klio server
	Client *ClientConfig `mapstructure:"client"`

	// KlioServerConfig is the configuration of the Klio server
	KlioServerConfig *KlioServerConfig `mapstructure:"klio_server"`
}

// SetDefaults sets the default values of the configuration.
func (d *Data) SetDefaults() {
	if d.Source != nil {
		d.Source.SetDefaults()
	}
}

// Source is the configuration of the WAL receiver.
type Source struct {
	// DSN is the database service we should get the WALs from
	DSN string `validate:"nonzero"`

	// Slot is the name of the replication slot to be used
	Slot string `validate:"nonzero"`

	// StandbyMessageTimeoutSeconds is the timeout after which the WAL
	// receiver will send a status update
	StandbyMessageTimeoutSeconds int `validate:"min=1"`
}

// ClientConfig is the configuration of the Klio server.
type ClientConfig struct {
	// Kopia is the configuration of the target Kopia repository
	Kopia *KopiaRepositoryClientConfig `mapstructure:"kopia"`

	// Klio is the configuration of the Klio repository
	Klio *KlioRepositoryClientConfig `mapstructure:"klio"`
}

// KlioRepositoryClientConfig is the configuration of the Klio repository
// where WALs should be uploaded.
type KlioRepositoryClientConfig struct {
	// Address of the Klio server
	Address string `validate:"nonzero"`

	// ClusterName is the name of the target cluster where to upload WALs
	ClusterName string `mapstructure:"cluster_name" validate:"nonzero"`

	// ServerKeyPath is the path to the server public key
	ServerCertPath string `mapstructure:"server_cert_path" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `mapstructure:"password" validate:"nonzero"`
}

// KopiaRepositoryClientConfig is the configuration of the Kopia repository
// to be used to upload the data directory.
type KopiaRepositoryClientConfig struct {
	// BaseURL is the base URL where the Kopia API server should be reached
	BaseURL string `mapstructure:"base_url" validate:"nonzero"`

	// TrustedServerCertificateFingerprint is used to authenticate to the server side
	TrustedServerCertificateFingerprint string `mapstructure:"trusted_server_certificate_fingerprint" validate:"nonzero"`

	// Hostname is the Klio server hostname.
	// This is used to create the full username, in the form <username>@<hostname>
	Hostname string `mapstructure:"hostname" validate:"nonzero"`

	// Username is the Klio server username.
	// This is used to create the full username, in the form <username>@<hostname>
	Username string `mapstructure:"username" validate:"nonzero"`

	// Password is the Klio server password
	Password string `mapstructure:"password" validate:"nonzero"`
}

// KlioServerConfig is the configuration of the Klio server.
type KlioServerConfig struct {
	// ListenAddress is the listening address
	ListenAddress string `mapstructure:"listen_address" validate:"nonzero"`

	// ServerKeyPath is the path to the server public key
	ServerCertPath string `mapstructure:"server_cert_path" validate:"nonzero"`

	// ServerKeyPath is the path to the server private key
	ServerKeyPath string `mapstructure:"server_key_path" validate:"nonzero"`

	// WALPath is the path where the WALs should be stored
	WALPath string `mapstructure:"wal_path" validate:"nonzero"`

	// PGDataPath is the path to the Kopia repo that is used to snapshot
	// PostgreSQL
	PGDataPath string `mapstructure:"wal_path" validate:"nonzero"`

	// Password is the encryption password
	Password string `mapstructure:"password" validate:"nonzero"`
}

// SetDefaults sets the default values of the configuration.
func (s *Source) SetDefaults() {
	s.StandbyMessageTimeoutSeconds = 10
}

// StandbyMessageTimeout returns the stanby message timeout in a
// time.Duration.
func (s *Source) StandbyMessageTimeout() time.Duration {
	return time.Second * time.Duration(s.StandbyMessageTimeoutSeconds)
}

package config

import "time"

// Data is the configuration.
//
// This struct is used to generate a secret in the Kubernetes cluster, so its serialization must be stable.
type Data struct {
	// Source is the configuration of the database we should collect WALs for.
	// This is only needed for the WAL pusher.
	Source SourceConfig `json:"source" mapstructure:"source"`

	// Client is the configuration of the Klio client
	Client ClientConfig `json:"client" mapstructure:"client"`

	// Tier1RetentionPolicy is the retention policy to be applied to tier1.
	Tier1RetentionPolicy *RetentionPolicy `json:"tier1_retention,omitempty" mapstructure:"retention"`

	// Tier2RetentionPolicy is the retention policy to be applied to tier2.
	Tier2RetentionPolicy *RetentionPolicy `json:"tier2_retention,omitempty" mapstructure:"tier2_retention"`
}

// SetDefaults sets the default values of the configuration.
func (d *Data) SetDefaults() {
	if d.Source != (SourceConfig{}) {
		d.Source.SetDefaults()
	}
}

// SourceConfig is the configuration of the WAL receiver.
type SourceConfig struct {
	// DSN is the database service we should get the WALs from
	DSN string `json:"dsn" mapstructure:"dsn"`

	// StandardDSN is the database service name to be used for a standard
	// database connection
	StandardDSN string `json:"standard_dsn" mapstructure:"standard_dsn"`

	// Slot is the name of the replication slot to be used
	Slot string `json:"slot" mapstructure:"slot"`

	// StandbyMessageTimeoutSeconds is the timeout after which the WAL
	// receiver will send a status update
	StandbyMessageTimeoutSeconds int `json:"standby_message_timeout_seconds" mapstructure:"standby_message_timeout_seconds"` //nolint:lll

	// FlushTimeoutMilliseconds is the timeout in milliseconds after which buffered
	// WAL data is automatically flushed to the Klio server
	FlushTimeoutMilliseconds int `json:"flush_timeout_ms" mapstructure:"flush_timeout_ms"`

	// BufferSize is the maximum size in bytes of the in-memory WAL buffer before
	// triggering an automatic flush
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size"`
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
	// Address of the Tier 1 Klio server
	Address string `json:"address" mapstructure:"address"`

	// Address of the Tier 2 Klio server
	Tier2Address string `json:"tier2_address" mapstructure:"tier2_address"`

	// ClusterName is the name of the target cluster where to upload WALs
	ClusterName string `json:"cluster_name" mapstructure:"cluster_name"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `json:"server_cert_path" mapstructure:"server_cert_path"`

	// ClientCertPath is the path to the client public key
	ClientCertPath string `json:"client_cert_path" mapstructure:"client_cert_path"`

	// ClientKeyPath is the path to the client private key
	ClientKeyPath string `json:"client_key_path" mapstructure:"client_key_path"`
}

// BaseRepositoryClientConfig is the configuration of the Kopia repository
// to be used to upload the data directory.
type BaseRepositoryClientConfig struct {
	// URL is the base URL where the Tier 1 Kopia API server should be reached
	URL string `json:"url" mapstructure:"url"`

	// URL is the base URL where the Tier 2 Kopia API server should be reached
	Tier2URL string `json:"tier2_url" mapstructure:"tier2_url"`

	// ServerCertPath is the path to the server public key
	ServerCertPath string `json:"server_cert_path" mapstructure:"server_cert_path"`

	// ClientCertPath is the path to the client public key
	ClientCertPath string `json:"client_cert_path" mapstructure:"client_cert_path"`

	// ClientKeyPath is the path to the client private key
	ClientKeyPath string `json:"client_key_path" mapstructure:"client_key_path"`
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

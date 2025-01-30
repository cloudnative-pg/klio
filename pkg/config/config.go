// Package config contains the configuration data structure and the relative
// helpers
package config

import "time"

// Data is the configuration.
type Data struct {
	// ClusterName is the name of the cluster
	ClusterName string `mapstructure:"cluster_name" validate:"nonzero"`

	// Source is the configuration of the database we should collect WALs for
	Source Source

	// Server is the configuration of the Klio server
	Server Server
}

// SetDefaults sets the default values of the configuration.
func (d *Data) SetDefaults() {
	d.Source.SetDefaults()
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

// Server is the configuration of the Klio server.
type Server struct {
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

// SetDefaults sets the default values of the configuration.
func (s *Source) SetDefaults() {
	s.StandbyMessageTimeoutSeconds = 10
}

// StandbyMessageTimeout returns the stanby message timeout in a
// time.Duration.
func (s *Source) StandbyMessageTimeout() time.Duration {
	return time.Second * time.Duration(s.StandbyMessageTimeoutSeconds)
}

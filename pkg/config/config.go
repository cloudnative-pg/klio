// Package config contains the configuration data structure and the relative
// helpers
package config

import "time"

// Data is the configuration
type Data struct {
	// ClusterName is the name of the cluster that will be stored
	ClusterName string `validate:"nonzero"`

	// Source is the configuration of the database we should collect WALs for
	Source Source

	// Tier1 is the configuration of the storage area we use to temporarily
	// collect data. It will be powered by a file system
	Tier1 LocalArea
}

// SetDefaults sets the default values of the configuration
func (d *Data) SetDefaults() {
	d.Source.SetDefaults()
}

// Source is the configuration of the WAL receiver
type Source struct {
	// DSN is the database service we should get the WALs from
	DSN string `validate:"nonzero"`

	// Slot is the name of the replication slot to be used
	Slot string `validate:"nonzero"`

	// StandbyMessageTimeoutSeconds is the timeout after which the WAL
	// receiver will send a status update
	StandbyMessageTimeoutSeconds int `validate:"min=1"`
}

// SetDefaults sets the default values of the configuration
func (s *Source) SetDefaults() {
	s.StandbyMessageTimeoutSeconds = 10
}

// StandbyMessageTimeout returns the stanby message timeout in a
// time.Duration
func (s *Source) StandbyMessageTimeout() time.Duration {
	return time.Second * time.Duration(s.StandbyMessageTimeoutSeconds)
}

// LocalArea is the configuration of the spool
type LocalArea struct {
	// Path is the path where the files will be stored
	Path string `validate:"nonzero"`

	// Password is the storage encryption key
	Password string `validate:"nonzero"`
}

// SetDefaults sets the default values of the configuration
func (s *LocalArea) SetDefaults() {
}

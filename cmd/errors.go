package cmd

import "errors"

var (
	// ErrKopiaClientSectionIsRequired is raised when the kopia client configuration is missing.
	ErrKopiaClientSectionIsRequired = errors.New("'client.kopia' configuration section is required")

	// ErrSourceSectionIsRequired is raised when the WAL pusher is started without a
	// source specification.
	ErrSourceSectionIsRequired = errors.New("'source' configuration section is required")

	// ErrClientSectionIsRequired is raired when the WAL pusher is started without a
	// client specification.
	ErrClientSectionIsRequired = errors.New("'client' configuration section is required")

	// ErrKlioClientSectionIsRequired is raised when the Klio client configuration is missing.
	ErrKlioClientSectionIsRequired = errors.New("'client.klio' configuration section is required")

	// ErrKlioServerSectionIsRequired is raised when the "klio_server" section is not present and the
	// Klio server is started.
	ErrKlioServerSectionIsRequired = errors.New("'klio_server' configuration section is required")
)

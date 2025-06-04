package cmd

import (
	"errors"
)

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
)

type invalidTablespaceRemapOptionError struct {
	Option string
}

func newInvalidTablespaceRemapOptionError(option string) *invalidTablespaceRemapOptionError {
	return &invalidTablespaceRemapOptionError{
		Option: option,
	}
}

func (e *invalidTablespaceRemapOptionError) Error() string {
	return "invalid tablespace remap option " + e.Option
}

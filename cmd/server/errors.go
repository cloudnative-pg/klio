package server

import "errors"

// ErrKlioServerSectionIsRequired is raised when the "server" section is not present and the
// Klio server is started.
var ErrKlioServerSectionIsRequired = errors.New("'server' configuration section is required")

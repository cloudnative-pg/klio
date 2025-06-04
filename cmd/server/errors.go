package server

import "errors"

// ErrKlioServerSectionIsRequired is raised when the "klio_server" section is not present and the
// Klio server is started.
var ErrKlioServerSectionIsRequired = errors.New("'klio_server' configuration section is required")

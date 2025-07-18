package walplayer

import _ "embed"

// buffer contains embedded binary data that serves as template content for generating
// fake WAL files. This data is repeatedly looped to create WAL files of any size
// with consistent, reproducible content for testing purposes.
//
//go:embed buffer
var buffer []byte

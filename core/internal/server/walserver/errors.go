package walserver

import "errors"

var (
	errReadOnly         = errors.New("read only repository")
	errEmptyClusterName = errors.New("empty cluster name")
	errEmptyWALName     = errors.New("empty WAL name")
	errEmptySegmentSize = errors.New("empty segment size")

	// ErrParsingClientCACertificate is raised when we couldn't parse
	// the client CA certificate file.
	ErrParsingClientCACertificate = errors.New("parsing client CA certificate file failed")
)

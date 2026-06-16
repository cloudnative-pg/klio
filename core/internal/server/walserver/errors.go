package walserver

import "errors"

var (
	errReadOnly         = errors.New("read only repository")
	errEmptyClusterName = errors.New("empty cluster name")
	errEmptyWALName     = errors.New("empty WAL name")
	errEmptySegmentSize = errors.New("empty segment size")

	// errNotWALSegment is returned when a file is not a real WAL segment
	// (e.g. .history, .backup or .partial) and therefore must not drive the
	// latest_written_* metrics.
	errNotWALSegment = errors.New("not a WAL segment")

	// ErrParsingClientCACertificate is raised when we couldn't parse
	// the client CA certificate file.
	ErrParsingClientCACertificate = errors.New("parsing client CA certificate file failed")
)

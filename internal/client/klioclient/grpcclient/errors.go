package grpcclient

import (
	"errors"
	"fmt"
)

// ErrInconsistentCertificate is raised when the server certificate cannot be parsed.
var ErrInconsistentCertificate = errors.New("inconsistent server certificate (parsing)")

// IncompleteWALFileError is raised when a WAL file has been uploaded incompletely.
type IncompleteWALFileError struct {
	uploadedSize uint64
	expectedSize uint64
}

// Error implements the error interface.
func (e *IncompleteWALFileError) Error() string {
	return fmt.Sprintf("uploaded %v expected %v", e.uploadedSize, e.expectedSize)
}

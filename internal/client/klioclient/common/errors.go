package common

import "fmt"

// WALNotFoundError is returned when the WAL file is not found.
type WALNotFoundError struct {
	WalName string
}

func (e *WALNotFoundError) Error() string {
	return fmt.Sprintf("WAL file not found: %s", e.WalName)
}

// ErrMissingWALFile is raised when the client requires a WAL file
// that doesn't exist on the server
var ErrMissingWALFile = fmt.Errorf("non existing WAL file")

// IncompleteTransmissionError is raised when downloading a WAL file
// from a Klio server and the transmission got interrupted after having
// received a correct block of data.
// This usually happens when a WAL file that is being written server-side
// is being read.
type IncompleteTransmissionError struct {
	// Inner is the underlying error
	Inner error

	// WrittenBytes is the number of bytes that have successfully beings received
	// by the server
	WrittenBytes uint64
}

// Error implements the error interface
func (e IncompleteTransmissionError) Error() string {
	return fmt.Sprintf("incomplete WAL file received: %s", e.Inner.Error())
}

// Unwrap implements the error interface
func (e IncompleteTransmissionError) Unwrap() error {
	return e.Inner
}

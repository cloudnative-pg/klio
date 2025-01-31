package common

import "fmt"

// WALNotFoundError is returned when the WAL file is not found.
type WALNotFoundError struct {
	WalName string
}

func (e *WALNotFoundError) Error() string {
	return fmt.Sprintf("WAL file not found: %s", e.WalName)
}

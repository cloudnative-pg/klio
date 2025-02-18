package sendwal

import "fmt"

// UnexpectedCopydataMessageError is raised from the WAL receiver got a CopyData message
// of unknown type.
type UnexpectedCopydataMessageError struct {
	messageLength int
	messageType   byte
}

// Error implements the error interface.
func (e *UnexpectedCopydataMessageError) Error() string {
	return fmt.Sprintf("unexpected copy data message, type=%v length=%v", e.messageType, e.messageLength)
}

// NewUnexpectedCopydataMessageError creates a new unexpected copy data message.
func NewUnexpectedCopydataMessageError(msg []byte) *UnexpectedCopydataMessageError {
	if len(msg) == 0 {
		return &UnexpectedCopydataMessageError{
			messageLength: 0,
			messageType:   0,
		}
	}

	return &UnexpectedCopydataMessageError{
		messageLength: len(msg),
		messageType:   msg[0],
	}
}

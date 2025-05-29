package sendwal

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgproto3"
)

// UnexpectedMessageError is raised from the WAL receiver got a CopyData message
// of unknown type.
type UnexpectedMessageError struct {
	msg pgproto3.BackendMessage
}

// NewUnexpectedMessageError creates a new unexpected copy data message.
func NewUnexpectedMessageError(msg pgproto3.BackendMessage) *UnexpectedMessageError {
	return &UnexpectedMessageError{
		msg: msg,
	}
}

// Error implements the error interface.
func (e *UnexpectedMessageError) Error() string {
	return fmt.Sprintf("unexpected message, type=%+v", e.msg)
}

// UnexpectedCopydataMessageError is raised from the WAL receiver got a CopyData message
// of unknown type.
type UnexpectedCopydataMessageError struct {
	messageLength int
	messageType   byte
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

// Error implements the error interface.
func (e *UnexpectedCopydataMessageError) Error() string {
	return fmt.Sprintf("unexpected copy data message, type=%v length=%v", e.messageType, e.messageLength)
}

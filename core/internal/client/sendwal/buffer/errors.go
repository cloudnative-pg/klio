package buffer

import "fmt"

// UnexpectedWalDataOffsetError is the error returned when
// the WAL data offset is not the expected one.
type UnexpectedWalDataOffsetError struct {
	offset   uint64
	expected uint64
}

func (e *UnexpectedWalDataOffsetError) Error() string {
	return fmt.Sprintf("Unexpected WAL data offset: %08x, expected: %08x", e.offset, e.expected)
}

// UnopenedFileForWALError is the error returned when a WAL
// record is received without a WAL file open.
type UnopenedFileForWALError struct {
	offset uint64
}

func (e *UnopenedFileForWALError) Error() string {
	return fmt.Sprintf("received write-ahead log record for offset %v with no file open", e.offset)
}

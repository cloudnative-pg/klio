package infrastructure

import "fmt"

// NoSingleResultSetError is returned when the number of result sets is not 1.
type NoSingleResultSetError struct {
	resultSets int
}

func (e *NoSingleResultSetError) Error() string {
	return fmt.Sprintf(
		"expected 1 result set from SHOW wal_segment_size, got %d",
		e.resultSets,
	)
}

// NoSingleRowError is returned when the number of result rows is not 1.
type NoSingleRowError struct {
	rows int
}

func (e *NoSingleRowError) Error() string {
	return fmt.Sprintf(
		"expected 1 result row from SHOW wal_segment_size, got %d",
		e.rows,
	)
}

// NoSingleColumnError is returned when the number of columns is not 1.
type NoSingleColumnError struct {
	columns int
}

func (e *NoSingleColumnError) Error() string {
	return fmt.Sprintf(
		"expected 1 result row from SHOW wal_segment_size, got %d",
		e.columns,
	)
}

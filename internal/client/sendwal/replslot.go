package sendwal

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pglogrepl"
	"github.com/jackc/pgx/v5/pgconn"
)

// ReadReplicationSlotParserError is raised the answer to  a READ_REPLICATION_SLOT
// query is not in the expected format.
type ReadReplicationSlotParserError struct {
	reason string
}

// Error implements the error interface.
func (e *ReadReplicationSlotParserError) Error() string {
	return e.reason
}

// NewReplicationSlotParserError creates a new ReplicationSlotParserError.
func NewReplicationSlotParserError(format string, args ...any) *ReadReplicationSlotParserError {
	return &ReadReplicationSlotParserError{
		reason: fmt.Sprintf(format, args...),
	}
}

// ParseReadReplicationSlotResult is the parsed result of the IDENTIFY_SYSTEM command.
type ParseReadReplicationSlotResult struct {
	SlotType   string
	RestartLSN pglogrepl.LSN
	RestartTLI int
}

// ReadReplicationSlot executes the IDENTIFY_SYSTEM command.
func ReadReplicationSlot(
	ctx context.Context,
	conn *pgconn.PgConn,
	slotName string,
) (ParseReadReplicationSlotResult, error) {
	sql := fmt.Sprintf("READ_REPLICATION_SLOT %s", slotName)
	return ParseReadReplicationSlot(conn.Exec(ctx, sql))
}

// ParseReadReplicationSlot parses the result of the IDENTIFY_SYSTEM command.
func ParseReadReplicationSlot(mrr *pgconn.MultiResultReader) (ParseReadReplicationSlotResult, error) {
	var rrs ParseReadReplicationSlotResult
	results, err := mrr.ReadAll()
	if err != nil {
		return rrs, err //nolint:wrapcheck
	}

	if len(results) != 1 {
		return rrs, NewReplicationSlotParserError("expected 1 result set, got %d", len(results))
	}

	result := results[0]
	if len(result.Rows) != 1 {
		return rrs, NewReplicationSlotParserError("expected 1 result row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if len(row) != 3 {
		return rrs, NewReplicationSlotParserError("expected 3 result columns, got %d", len(row))
	}

	rrs.SlotType = string(row[0])

	if len(row[1]) > 0 {
		rrs.RestartLSN, err = pglogrepl.ParseLSN(string(row[1]))
		if err != nil {
			return rrs, NewReplicationSlotParserError("failed to parse timeline: %v", err)
		}
	}

	if len(row[2]) > 0 {
		timeline, err := strconv.ParseInt(string(row[2]), 10, 32)
		if err != nil {
			return rrs, NewReplicationSlotParserError("failed to parse timeline: %v", err)
		}

		rrs.RestartTLI = int(timeline)
	}

	return rrs, nil
}

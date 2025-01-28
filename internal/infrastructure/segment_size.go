package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

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

func (s *impl) GetWalSegmentSize(ctx context.Context) (uint64, error) {
	conn, err := pgconn.Connect(ctx, s.config.Source.DSN)
	if err != nil {
		return 0, fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			s.logger.ErrorContext(ctx, "Error while closing the connection")
		}
	}()
	mrr := conn.Exec(ctx, "SHOW wal_segment_size")

	results, err := mrr.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("could not read wal_segment_size: %w", err)
	}

	if len(results) != 1 {
		return 0, &NoSingleResultSetError{len(results)}
	}

	result := results[0]
	if len(result.Rows) != 1 {
		return 0, &NoSingleRowError{len(result.Rows)}
	}

	row := result.Rows[0]
	if len(row) != 1 {
		return 0, &NoSingleColumnError{len(row)}
	}

	res, err := parseWALSegmentSize(string(row[0]))
	if err != nil {
		return 0, err
	}

	s.logger.Info(
		"Detected WAL segment size",
		"walSegmentSize", res,
	)

	return res, nil
}

func parseWALSegmentSize(size string) (uint64, error) {
	parseWithMultiplier := func(size string, multiplier uint64) (uint64, error) {
		v, err := strconv.ParseUint(size, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("while parsing size '%s': %w", size, err)
		}

		return v * multiplier, nil
	}

	switch {
	case strings.HasSuffix(size, "KB"):
		return parseWithMultiplier(size[0:len(size)-2], 1024)
	case strings.HasSuffix(size, "MB"):
		return parseWithMultiplier(size[0:len(size)-2], 1024*1024)
	case strings.HasSuffix(size, "GB"):
		return parseWithMultiplier(size[0:len(size)-2], 1024*1024*1024)
	default:
		return parseWithMultiplier(size, 1)
	}
}

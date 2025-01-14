package receiver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *impl) getWALSegmentSize(ctx context.Context, conn *pgconn.PgConn) (uint64, error) {
	mrr := conn.Exec(ctx, "SHOW wal_segment_size")

	results, err := mrr.ReadAll()
	if err != nil {
		return 0, err
	}

	if len(results) != 1 {
		return 0, fmt.Errorf(
			"expected 1 result set from SHOW wal_segment_size, got %d",
			len(results),
		)
	}

	result := results[0]
	if len(result.Rows) != 1 {
		return 0, fmt.Errorf(
			"expected 1 result row from SHOW wal_segment_size, got %d",
			len(result.Rows),
		)
	}

	row := result.Rows[0]
	if len(row) != 1 {
		return 0, fmt.Errorf(
			"expected 1 result column from SHOW wal_segment_size, got %d",
			len(row),
		)
	}

	return parseWALSegmentSize(string(row[0]))
}

func parseWALSegmentSize(size string) (uint64, error) {
	parseWithMultiplier := func(size string, multiplier uint64) (uint64, error) {
		v, err := strconv.ParseUint(size, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("while parsing size '%s': %w", size, err)
		}
		return v * multiplier, err
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

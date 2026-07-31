/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// GetWalSegmentSize returns the size of the WAL segment.
func (s *Postgres) GetWalSegmentSize(ctx context.Context) (uint64, error) {
	conn, err := pgconn.Connect(ctx, s.config.Source.DSN)
	if err != nil {
		return 0, fmt.Errorf("while parsing DSN: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(ctx); closeErr != nil {
			s.logger.Error(closeErr, "Error while closing the connection")
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

	const multiplier = 1024
	switch {
	case strings.HasSuffix(size, "KB"):
		return parseWithMultiplier(size[0:len(size)-2], multiplier)
	case strings.HasSuffix(size, "MB"):
		return parseWithMultiplier(size[0:len(size)-2], multiplier*multiplier)
	case strings.HasSuffix(size, "GB"):
		return parseWithMultiplier(size[0:len(size)-2], multiplier*multiplier*multiplier)
	default:
		return parseWithMultiplier(size, 1)
	}
}

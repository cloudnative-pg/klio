package buffer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudnative-pg/machinery/pkg/types"
)

// Data is the implementation of the WAL buffer.
type Data struct {
	segmentSize uint64
	tli         int
	logger      *slog.Logger

	handler Handler
}

// New creates a new WAL buffer.
func New(logger *slog.Logger, tli int, walSegmentSize uint64, handler Handler) *Data {
	return &Data{
		segmentSize: walSegmentSize,
		tli:         tli,
		handler:     handler,
		logger:      logger,
	}
}

// ProcessWALData processes a WAL message from PG
//
//nolint:cyclop
func (wal *Data) ProcessWALData(ctx context.Context, data []byte, startWAL types.LSN) error {
	// This implementation is largely based on src/bin/pg_basebackup/receivelog.c
	// [ProcessXLogDataMsg]

	//nolint:lll
	// See: https://github.com/postgres/postgres/blob/00f4c2959d631c7851da21a512885d1deab28649/src/bin/pg_basebackup/receivelog.c#L1039

	wal.logger.Debug("Process WAL Data", "lenData", len(data), "startWAL", startWAL)

	blockpos, err := startWAL.Parse()
	if err != nil {
		return fmt.Errorf("while parsing WAL data start (pos): %w", err)
	}

	xlogoff := blockpos % wal.segmentSize

	if !wal.handler.HasWALFileOpened() {
		if xlogoff != 0 {
			// No file open yet
			return &UnopenedFileForWALError{offset: xlogoff}
		}
	} else {
		// More data in existing segment
		if wal.handler.CurrentOffset() != xlogoff {
			return &UnexpectedWalDataOffsetError{
				offset:   xlogoff,
				expected: wal.handler.CurrentOffset(),
			}
		}
	}

	bytesLeft := uint64(len(data))
	bytesWritten := uint64(0)
	for bytesLeft > 0 {
		var bytesToWrite uint64

		// If crossing a WAL boundary, only write up until we reach wal
		// segment size.
		if xlogoff+bytesLeft > wal.segmentSize {
			bytesToWrite = wal.segmentSize - xlogoff
		} else {
			bytesToWrite = bytesLeft
		}

		if !wal.handler.HasWALFileOpened() {
			if err := wal.handler.OpenWAL(ctx, blockpos); err != nil {
				return err //nolint:wrapcheck
			}
		}

		_, err := wal.handler.Write(ctx, data[bytesWritten:bytesWritten+bytesToWrite])
		if err != nil {
			return fmt.Errorf("while writing to WAL handler: %w", err)
		}

		bytesWritten += bytesToWrite
		bytesLeft -= bytesToWrite
		blockpos += bytesToWrite
		xlogoff += bytesToWrite

		// Did we reach the end of a WAL segment?
		if wal.handler.CurrentOffset() == wal.segmentSize {
			err := wal.handler.CloseWAL(ctx)
			if err != nil {
				return fmt.Errorf("while flushing WAL handler: %w", err)
			}

			xlogoff = 0
		}
	}

	return nil
}

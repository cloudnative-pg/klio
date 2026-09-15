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

package buffer

import (
	"bytes"
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/jackc/pglogrepl"
)

// maximumBufferSizeFactor allows configuring the higher limit of memory
// allocation of the WAL buffer. It is multiplied to the configured
// buffer size to get the limit.
const maximumBufferSizeFactor = 2

// Data is the implementation of the WAL buffer.
type Data struct {
	segmentSize uint64
	tli         int

	walHandler WALHandler

	localWriteLSN uint64

	buffer     *bytes.Buffer
	bufferSize int
}

// New creates a new WAL buffer.
func New(tli int, walSegmentSize uint64, handler WALHandler, bufferSize int) *Data {
	result := &Data{
		segmentSize: walSegmentSize,
		tli:         tli,
		walHandler:  handler,
		bufferSize:  bufferSize,
	}

	result.buffer = result.newBuffer()

	return result
}

// ProcessWALData processes a WAL message from PG
//
//nolint:cyclop
func (wal *Data) ProcessWALData(ctx context.Context, data []byte, blockpos pglogrepl.LSN) error {
	// This implementation is largely based on src/bin/pg_basebackup/receivelog.c
	// [ProcessXLogDataMsg]

	//nolint:lll
	// See: https://github.com/postgres/postgres/blob/00f4c2959d631c7851da21a512885d1deab28649/src/bin/pg_basebackup/receivelog.c#L1039

	xlogoff := uint64(blockpos) % wal.segmentSize

	if !wal.walHandler.HasWALFileOpened() {
		if xlogoff != 0 {
			// No file open yet
			return &UnopenedFileForWALError{offset: xlogoff}
		}
	} else {
		// More data in existing segment
		currentOffset := wal.localWriteLSN % wal.segmentSize
		if currentOffset != xlogoff {
			return &UnexpectedWalDataOffsetError{
				offset:   xlogoff,
				expected: currentOffset,
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

		if !wal.walHandler.HasWALFileOpened() {
			if err := wal.openWALPos(ctx, uint64(blockpos)); err != nil {
				return err
			}
		}

		if err := wal.writeToWALFile(data[bytesWritten : bytesWritten+bytesToWrite]); err != nil {
			return fmt.Errorf("while writing to WAL handler: %w", err)
		}

		bytesWritten += bytesToWrite
		bytesLeft -= bytesToWrite
		blockpos += pglogrepl.LSN(bytesToWrite)
		xlogoff += bytesToWrite

		// Did we reach the end of a WAL segment?
		if currentOffset := wal.localWriteLSN % wal.segmentSize; currentOffset == 0 {
			if err := wal.CloseCurrentWAL(ctx); err != nil {
				return err
			}

			xlogoff = 0
		}
	}

	return nil
}

// Flush writes any buffered data to the currently open WAL file. It is a
// no-op when no WAL file is open or the buffer is empty.
func (wal *Data) Flush(ctx context.Context) error {
	if wal.walHandler == nil || !wal.walHandler.HasWALFileOpened() || wal.buffer.Len() == 0 {
		return nil
	}

	_, err := wal.walHandler.Write(ctx, wal.buffer.Bytes())
	if err != nil {
		return fmt.Errorf("while writing to WAL handler: %w", err)
	}

	// Clear content but keeps the slice capacity
	wal.buffer.Reset()

	// Prevent memory bloat in long-running processes.
	if wal.buffer.Cap() > wal.bufferSize*maximumBufferSizeFactor {
		wal.buffer = wal.newBuffer()
	}

	return nil
}

// CloseCurrentWAL flushes and closes the currently open WAL file. It is a
// no-op when no WAL file is open.
func (wal *Data) CloseCurrentWAL(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Debug("Closing WAL file")
	if !wal.walHandler.HasWALFileOpened() {
		return nil
	}

	if err := wal.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing WAL handler: %w", err)
	}

	if err := wal.walHandler.CloseWAL(ctx); err != nil {
		return fmt.Errorf("while closing current WAL file: %w", err)
	}

	return nil
}

func (wal *Data) newBuffer() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, wal.bufferSize))
}

func (wal *Data) openWALPos(ctx context.Context, blockpos uint64) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Opening WAL file", "blockpos", types.Int64ToLSN(blockpos))

	if err := wal.walHandler.OpenWAL(ctx, blockpos); err != nil {
		return err //nolint:wrapcheck
	}

	wal.localWriteLSN = blockpos

	return nil
}

// writeToWALFile appends data to the in-memory buffer. It does not flush:
// like a PostgreSQL walreceiver, which writes whatever it received and
// flushes once per receive cycle rather than accumulating to a target size,
// flushing here is left entirely to the caller (once per drained batch of
// messages, or at segment close) so a block sent to the Klio server tracks
// how much WAL actually arrived at once, not an unrelated size threshold.
func (wal *Data) writeToWALFile(data []byte) error {
	if _, err := wal.buffer.Write(data); err != nil {
		return fmt.Errorf("while writing to buffer: %w", err)
	}

	wal.localWriteLSN += uint64(len(data))

	return nil
}

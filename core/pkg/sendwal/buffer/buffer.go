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
)

// maximumBufferSizeFactor allows configuring the higher limit of memory
// allocation of the WAL buffer. It is multiplied to the configured
// buffer size to get the limit.
const maximumBufferSizeFactor = 2

// Data is the implementation of the WAL buffer.
type Data struct {
	segmentSize uint64
	tli         int

	handler Handler

	writeLSN   uint64
	flushLSN   uint64
	buffer     *bytes.Buffer
	bufferSize int
}

// New creates a new WAL buffer.
func New(tli int, walSegmentSize uint64, handler Handler, bufferSize int) *Data {
	result := &Data{
		segmentSize: walSegmentSize,
		tli:         tli,
		handler:     handler,
		bufferSize:  bufferSize,
	}

	result.buffer = result.newBuffer()

	return result
}

// ProcessWALData processes a WAL message from PG
//
//nolint:cyclop
func (wal *Data) ProcessWALData(ctx context.Context, data []byte, startWAL types.LSN) error {
	contextLogger := log.FromContext(ctx)

	// This implementation is largely based on src/bin/pg_basebackup/receivelog.c
	// [ProcessXLogDataMsg]

	//nolint:lll
	// See: https://github.com/postgres/postgres/blob/00f4c2959d631c7851da21a512885d1deab28649/src/bin/pg_basebackup/receivelog.c#L1039

	contextLogger.Debug("Process WAL Data", "lenData", len(data), "startWAL", startWAL)

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
		currentOffset := wal.writeLSN % wal.segmentSize
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

		if !wal.handler.HasWALFileOpened() {
			if err := wal.openWALPos(ctx, blockpos); err != nil {
				return err
			}
		}

		if err := wal.writeToWALFile(ctx, data[bytesWritten:bytesWritten+bytesToWrite]); err != nil {
			return fmt.Errorf("while writing to WAL handler: %w", err)
		}

		bytesWritten += bytesToWrite
		bytesLeft -= bytesToWrite
		blockpos += bytesToWrite
		xlogoff += bytesToWrite

		// Did we reach the end of a WAL segment?
		if currentOffset := wal.writeLSN % wal.segmentSize; currentOffset == 0 {
			if err := wal.closeCurrentWAL(ctx); err != nil {
				return err
			}

			xlogoff = 0
		}
	}

	return nil
}

// FlushLSN gets the latest LSN that was flushed down to the destination.
func (wal *Data) FlushLSN() uint64 {
	return wal.flushLSN
}

// WriteLSN gets the latest LSN that was written into the memory.
func (wal *Data) WriteLSN() uint64 {
	return wal.writeLSN
}

// Flush flushes the buffer to the underlying handler.
func (wal *Data) Flush(ctx context.Context) error {
	return wal.flushInternal(ctx)
}

func (wal *Data) newBuffer() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, wal.bufferSize))
}

func (wal *Data) openWALPos(ctx context.Context, blockpos uint64) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Info("Opening WAL file", "blockpos", types.Int64ToLSN(blockpos))

	if err := wal.handler.OpenWAL(ctx, blockpos); err != nil {
		return err //nolint:wrapcheck
	}

	wal.writeLSN = blockpos
	wal.flushLSN = blockpos

	return nil
}

func (wal *Data) writeToWALFile(ctx context.Context, data []byte) error {
	if _, err := wal.buffer.Write(data); err != nil {
		return fmt.Errorf("while writing to buffer: %w", err)
	}

	wal.writeLSN += uint64(len(data))

	if wal.buffer.Len() >= wal.bufferSize {
		return wal.Flush(ctx)
	}

	return nil
}

func (wal *Data) flushInternal(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	if wal.handler == nil || !wal.handler.HasWALFileOpened() || wal.buffer.Len() == 0 {
		return nil
	}

	contextLogger.Debug("Writing block",
		"blockpos", types.Int64ToLSN(wal.writeLSN), "blocksize", wal.buffer.Len())
	_, err := wal.handler.Write(ctx, wal.buffer.Bytes())
	if err != nil {
		return fmt.Errorf("while writing to WAL handler: %w", err)
	}

	// Clear content but keeps the slice capacity
	wal.buffer.Reset()

	// Prevent memory bloat in long-running processes.
	if wal.buffer.Cap() > wal.bufferSize*maximumBufferSizeFactor {
		wal.buffer = wal.newBuffer()
	}

	wal.flushLSN = wal.writeLSN

	return nil
}

func (wal *Data) closeCurrentWAL(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)
	contextLogger.Debug("Closing WAL file")

	if err := wal.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing WAL handler: %w", err)
	}

	if err := wal.handler.CloseWAL(ctx); err != nil {
		return fmt.Errorf("while closing current WAL file: %w", err)
	}

	return nil
}

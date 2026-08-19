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

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"
)

// Flusher is the type of functions that are called
// to write a WAL file.
type Flusher func(walName string, data []byte) error

// MemBufferHandler is the handler of WAL files that writes in a memory buffer
// and, when the WAL is completed, flushes it via a Flusher function.
type MemBufferHandler struct {
	currentWALFile string
	buffer         bytes.Buffer
	logger         log.Logger
	flusher        Flusher

	tli         int
	segmentSize uint64
}

// NewMemBufferHandler creates a new memory buffer handler.
func NewMemBufferHandler(logger log.Logger, tli int, segmentSize uint64, flusher Flusher) *MemBufferHandler {
	return &MemBufferHandler{
		currentWALFile: "",
		buffer:         *bytes.NewBuffer(make([]byte, 0, segmentSize)),
		logger:         logger,
		flusher:        flusher,
		tli:            tli,
		segmentSize:    segmentSize,
	}
}

// HasWALFileOpened implements the Handler interface.
func (wal *MemBufferHandler) HasWALFileOpened() bool {
	return wal.currentWALFile != ""
}

// OpenWAL implements the Handler interface.
func (wal *MemBufferHandler) OpenWAL(_ context.Context, blockpos uint64) error {
	var err error

	wal.currentWALFile, err = types.Int64ToLSN(blockpos).WALFileName(wal.tli, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}
	wal.buffer.Reset()

	wal.logger.Debug("Opening WAL File", "walFileName", wal.currentWALFile)

	return nil
}

// CloseWAL implements the Handler interface.
func (wal *MemBufferHandler) CloseWAL(_ context.Context) error {
	wal.logger.Debug("Closing WAL File", "walFileName", wal.currentWALFile)
	if err := wal.flusher(wal.currentWALFile, wal.buffer.Bytes()); err != nil {
		return err
	}

	wal.currentWALFile = ""
	wal.buffer.Reset()

	return nil
}

// CurrentOffset implements the Handler interface.
func (wal *MemBufferHandler) CurrentOffset() (uint64, error) {
	return safecast.Convert[uint64](wal.buffer.Len())
}

// Write implements the Handler interface.
func (wal *MemBufferHandler) Write(_ context.Context, p []byte) (int, error) {
	return wal.buffer.Write(p) //nolint:wrapcheck
}

// SyncedOffset implements the Handler interface. The in-memory handler has no
// remote durability round-trip, so the synced offset equals the written one.
func (wal *MemBufferHandler) SyncedOffset() (uint64, error) {
	return wal.CurrentOffset()
}

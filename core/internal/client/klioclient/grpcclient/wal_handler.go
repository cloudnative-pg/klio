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

package grpcclient

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	sendwalbuffer "github.com/cloudnative-pg/klio/core/pkg/sendwal/buffer"
)

// KlioClientStreamingHandler is a sendwalbuffer.Handler that streams WAL
// data directly to a Klio server as it is received, rather than buffering
// a whole WAL file before sending it.
type KlioClientStreamingHandler struct {
	conn *Connection

	stream klioclient.WALUploaderImpl
	offset uint64

	sendToTier2 bool

	tli            int
	segmentSize    uint64
	currentWALFile string
}

// NewKlioClientHandler creates a new klio handler.
func NewKlioClientHandler(
	tli int,
	segmentSize uint64,
	conn *Connection,
	sendToTier2 bool,
) *KlioClientStreamingHandler {
	return &KlioClientStreamingHandler{
		conn:        conn,
		tli:         tli,
		segmentSize: segmentSize,
		stream:      nil,
		sendToTier2: sendToTier2,
	}
}

// NewKlioClientHandlerFactory returns a sendwal.HandlerFactory
// (github.com/cloudnative-pg/klio/core/pkg/sendwal) building
// KlioClientStreamingHandler instances that stream to conn.
func NewKlioClientHandlerFactory(
	conn *Connection,
	sendToTier2 bool,
) func(tli int, segmentSize uint64) sendwalbuffer.Handler {
	return func(tli int, segmentSize uint64) sendwalbuffer.Handler {
		return NewKlioClientHandler(tli, segmentSize, conn, sendToTier2)
	}
}

// OpenWAL implements the buffer.Handler interface.
func (wal *KlioClientStreamingHandler) OpenWAL(ctx context.Context, blockpos uint64) error {
	currentWALFile, err := types.Int64ToLSN(blockpos).WALFileName(wal.tli, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}

	wal.offset = 0
	wal.currentWALFile = currentWALFile

	stream, err := wal.conn.StoreWALStreaming(ctx, wal.currentWALFile, wal.segmentSize, wal.sendToTier2)
	if err != nil {
		return fmt.Errorf("while starting WAL file streaming (pos %v): %w", blockpos, err)
	}

	wal.stream = stream

	return nil
}

// HasWALFileOpened implements the buffer.Handler interface.
func (wal *KlioClientStreamingHandler) HasWALFileOpened() bool {
	return wal.currentWALFile != ""
}

// CloseWAL implements the buffer.Handler interface.
func (wal *KlioClientStreamingHandler) CloseWAL(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	contextLogger.Debug("Closing WAL File", "walFileName", wal.currentWALFile)

	if err := wal.stream.Close(ctx); err != nil {
		return err //nolint:wrapcheck
	}

	wal.currentWALFile = ""
	wal.stream = nil

	return nil
}

// CurrentOffset implements the buffer.Handler interface.
func (wal *KlioClientStreamingHandler) CurrentOffset() (uint64, error) {
	return wal.offset, nil
}

// Write implements the buffer.Handler interface.
func (wal *KlioClientStreamingHandler) Write(ctx context.Context, block []byte) (int, error) {
	err := wal.stream.SendBlock(ctx, block)
	if err != nil {
		return 0, err //nolint:wrapcheck
	}

	wal.offset += uint64(len(block))

	return len(block), nil
}

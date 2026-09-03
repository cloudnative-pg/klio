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
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
	"github.com/cloudnative-pg/klio/core/internal/wal"
)

// KlioClientTimelineHandler is a handler that streams directly to a
// Klio server.
type KlioClientTimelineHandler struct {
	conn *grpcclient.Connection

	stream          klioclient.WALUploader
	feedbackChannel chan<- wal.Feedback

	sendToTier2 bool

	tli            int
	segmentSize    uint64
	currentWALFile string
}

// NewKlioClientTimelineHandler creates a new klio handler.
func NewKlioClientTimelineHandler(
	tli int,
	segmentSize uint64,
	conn *grpcclient.Connection,
	sendToTier2 bool,
	feedbackChannel chan<- wal.Feedback,
) *KlioClientTimelineHandler {
	return &KlioClientTimelineHandler{
		conn:            conn,
		tli:             tli,
		segmentSize:     segmentSize,
		stream:          nil,
		sendToTier2:     sendToTier2,
		feedbackChannel: feedbackChannel,
	}
}

// OpenWAL implements the WALHandler interface.
func (timelineHandler *KlioClientTimelineHandler) OpenWAL(ctx context.Context, blockpos uint64) error {
	currentWALFile, err := types.Int64ToLSN(blockpos).WALFileName(timelineHandler.tli, timelineHandler.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}

	timelineHandler.currentWALFile = currentWALFile

	// We are starting a new WAL: report progress up to its start position
	// right away, rather than waiting for the Klio server to ack any of its
	// blocks. Otherwise PG would see our reported LSNs stall at the end of
	// the previous file for as long as the new file's first ack takes to
	// arrive, which can look like the standby has stopped progressing.
	timelineHandler.feedbackChannel <- wal.Feedback{
		FlushLSN:  blockpos,
		WriteLSN:  blockpos,
		ReplayLSN: blockpos,
	}

	stream, err := timelineHandler.conn.StoreWALStreaming(
		ctx,
		timelineHandler.currentWALFile,
		timelineHandler.segmentSize,
		timelineHandler.sendToTier2,
		blockpos,
		timelineHandler.feedbackChannel,
	)
	if err != nil {
		return fmt.Errorf("while starting WAL file streaming (pos %v): %w", blockpos, err)
	}

	timelineHandler.stream = stream

	return nil
}

// HasWALFileOpened implements the WALHandler interface.
func (timelineHandler *KlioClientTimelineHandler) HasWALFileOpened() bool {
	return timelineHandler.currentWALFile != ""
}

// CloseWAL implements the WALHandler interface.
func (timelineHandler *KlioClientTimelineHandler) CloseWAL(ctx context.Context) error {
	contextLogger := log.FromContext(ctx)

	contextLogger.Debug("Closing WAL File", "walFileName", timelineHandler.currentWALFile)

	if err := timelineHandler.stream.Close(ctx); err != nil {
		return err //nolint:wrapcheck
	}

	timelineHandler.currentWALFile = ""
	timelineHandler.stream = nil

	return nil
}

// maxWriteChunkBytes caps how much of a Write call is handed to a single
// SendBlock call. buffer.Data no longer caps how much it accumulates before
// flushing (it flushes at whatever granularity WAL was received in, like a
// PostgreSQL walreceiver), so a single Write here may carry more than one
// gRPC message is allowed to hold - the server rejects anything over
// wal.MaxBlockSizeBytes (8 MiB); this stays safely under that.
const maxWriteChunkBytes = 4 * 1024 * 1024

// Write implements the WALHandler interface. It splits block into chunks of
// at most maxWriteChunkBytes, each sent with its own SendBlock call, so that
// a single Write never hands gRPC a message over the server's accepted size
// (see maxWriteChunkBytes). As a side effect, acks - delivered asynchronously
// on feedbackChannel by grpcWALStream's background reader - can arrive
// progressively across chunks rather than only once at the very end, though
// the server may still coalesce several chunks into a single flush and
// acknowledgment (see blockReceiver.Drain).
func (timelineHandler *KlioClientTimelineHandler) Write(ctx context.Context, block []byte) (int, error) {
	written := 0

	for len(block) > 0 {
		chunk := block
		if len(chunk) > maxWriteChunkBytes {
			chunk = chunk[:maxWriteChunkBytes]
		}

		if err := timelineHandler.stream.SendBlock(ctx, chunk); err != nil {
			return written, err //nolint:wrapcheck
		}

		written += len(chunk)
		block = block[len(chunk):]
	}

	return written, nil
}

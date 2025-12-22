package buffer

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/types"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/grpcclient"
)

// KlioClientStreamingHandler is a handler that streams directly to a
// Klio server.
type KlioClientStreamingHandler struct {
	conn *grpcclient.Connection

	stream klioclient.WALUploaderImpl
	offset uint64

	tli            int
	segmentSize    uint64
	currentWALFile string
}

// NewKlioClientHandler creates a new klio handler.
func NewKlioClientHandler(
	tli int,
	segmentSize uint64,
	conn *grpcclient.Connection,
) *KlioClientStreamingHandler {
	return &KlioClientStreamingHandler{
		conn:        conn,
		tli:         tli,
		segmentSize: segmentSize,
		stream:      nil,
	}
}

// OpenWAL implements the Handler interface.
func (wal *KlioClientStreamingHandler) OpenWAL(ctx context.Context, blockpos uint64) error {
	currentWALFile, err := types.Int64ToLSN(blockpos).WALFileName(wal.tli, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}

	wal.offset = 0
	wal.currentWALFile = currentWALFile

	stream, err := wal.conn.StoreWALStreaming(ctx, wal.currentWALFile, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while starting WAL file streaming (pos %v): %w", blockpos, err)
	}

	wal.stream = stream

	return nil
}

// HasWALFileOpened implements the Handler interface.
func (wal *KlioClientStreamingHandler) HasWALFileOpened() bool {
	return wal.currentWALFile != ""
}

// CloseWAL implements the Handler interface.
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

// CurrentOffset implements the Handler interface.
func (wal *KlioClientStreamingHandler) CurrentOffset() (uint64, error) {
	return wal.offset, nil
}

// Write implements the Handler interface.
func (wal *KlioClientStreamingHandler) Write(ctx context.Context, block []byte) (int, error) {
	err := wal.stream.SendBlock(ctx, block)
	if err != nil {
		return 0, err //nolint:wrapcheck
	}

	wal.offset += uint64(len(block))

	return len(block), nil
}

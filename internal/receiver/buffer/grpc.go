package buffer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cloudnative-pg/machinery/pkg/types"

	"github.com/EnterpriseDB/klio/pkg/klioclient/common"
)

// KlioClientStreamingHandler is a handler that streams directly to a
// Klio server.
type KlioClientStreamingHandler struct {
	conn   common.WALClientStreamer
	logger *slog.Logger

	stream common.WALUploaderImpl
	offset uint64

	tli            int
	segmentSize    uint64
	currentWALFile string
}

// NewKlioClientHandler creates a new klio handler.
func NewKlioClientHandler(
	logger *slog.Logger,
	tli int,
	segmentSize uint64,
	conn common.WALClientStreamer,
) *KlioClientStreamingHandler {
	return &KlioClientStreamingHandler{
		logger:      logger,
		conn:        conn,
		tli:         tli,
		segmentSize: segmentSize,
		stream:      nil,
	}
}

// OpenWAL implements the Handler interface.
func (wal *KlioClientStreamingHandler) OpenWAL(blockpos uint64) error {
	currentWALFile, err := types.Int64ToLSN(blockpos).WALFileName(wal.tli, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while creating WAL file name (pos %v): %w", blockpos, err)
	}

	stream, err := wal.conn.StoreWALStreaming(context.TODO(), wal.currentWALFile, wal.segmentSize)
	if err != nil {
		return fmt.Errorf("while starting WAL file streaming (pos %v): %w", blockpos, err)
	}

	wal.offset = 0
	wal.currentWALFile = currentWALFile
	wal.stream = stream

	return nil
}

// HasWALFileOpened implements the Handler interface.
func (wal *KlioClientStreamingHandler) HasWALFileOpened() bool {
	return wal.currentWALFile != ""
}

// CloseWAL implements the Handler interface.
func (wal *KlioClientStreamingHandler) CloseWAL() error {
	wal.logger.Debug("Closing WAL File", "walFileName", wal.currentWALFile)

	if err := wal.stream.Close(context.TODO()); err != nil {
		return err //nolint:wrapcheck
	}

	wal.currentWALFile = ""
	wal.stream = nil

	return nil
}

// CurrentOffset implements the Handler interface.
func (wal *KlioClientStreamingHandler) CurrentOffset() uint64 {
	return wal.offset
}

// Write implements the Handler interface.
func (wal *KlioClientStreamingHandler) Write(block []byte) (int, error) {
	err := wal.stream.SendBlock(context.TODO(), block)
	if err != nil {
		return 0, err //nolint:wrapcheck
	}

	wal.offset += uint64(len(block))

	return len(block), nil
}

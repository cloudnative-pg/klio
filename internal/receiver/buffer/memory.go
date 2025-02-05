package buffer

import (
	"bytes"
	"fmt"
	"log/slog"

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
	logger         *slog.Logger
	flusher        Flusher

	tli         int
	segmentSize uint64
}

// NewMemBufferHandler creates a new memory buffer handler.
func NewMemBufferHandler(logger *slog.Logger, tli int, segmentSize uint64, flusher Flusher) *MemBufferHandler {
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
func (wal *MemBufferHandler) OpenWAL(blockpos uint64) error {
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
func (wal *MemBufferHandler) CloseWAL() error {
	wal.logger.Debug("Closing WAL File", "walFileName", wal.currentWALFile)
	if err := wal.flusher(wal.currentWALFile, wal.buffer.Bytes()); err != nil {
		return err
	}

	wal.currentWALFile = ""
	wal.buffer.Reset()

	return nil
}

// CurrentOffset implements the Handler interface.
func (wal *MemBufferHandler) CurrentOffset() uint64 {
	return uint64(wal.buffer.Len())
}

// Write implements the Handler interface.
func (wal *MemBufferHandler) Write(p []byte) (int, error) {
	return wal.buffer.Write(p) //nolint:wrapcheck
}

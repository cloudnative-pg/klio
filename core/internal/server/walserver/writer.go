package walserver

import (
	"fmt"
	"os"
	"path"

	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
)

// Writer is the repository file writer.
type Writer struct {
	walFilePath        string
	walFilePartialPath string
	conn               *repository.Connection

	file *os.File
}

// NewWriter creates a new WAL file writer.
func NewWriter(conn *repository.Connection, clusterName, walName string, segmentSize uint64) (*Writer, error) {
	walFilePath := getWALArchivePath(conn.BaseDir(), clusterName, walName)
	walFilePartialPath := walFilePath + ".partial"

	// Step 1: ensure the parent path exists
	err := os.MkdirAll(path.Dir(walFilePath), 0o750)
	if err != nil {
		return nil, fmt.Errorf(
			"error while creating directory %s: %w",
			path.Base(walFilePath),
			err,
		)
	}

	// Step 2: open the file
	//nolint:gosec
	file, err := os.OpenFile(walFilePartialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_SYNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			walFilePath,
			err,
		)
	}

	startBlock := grpc.StartWALFile{
		KlioVersion: 1,
		FileLength:  segmentSize,
	}

	if _, err := protodelim.MarshalTo(file, &startBlock); err != nil {
		return nil, fmt.Errorf("while writing WAL file header: %w", err)
	}

	return &Writer{
		walFilePath:        walFilePath,
		walFilePartialPath: walFilePartialPath,
		file:               file,
		conn:               conn,
	}, nil
}

// CloseMarkDone closes the WAL writer and mark the file as completed.
func (w *Writer) CloseMarkDone() error {
	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing partial file: %w", err)
	}

	if err := os.Rename(w.walFilePartialPath, w.walFilePath); err != nil {
		return fmt.Errorf("while renaming partial file: %w", err)
	}

	return nil
}

// Flush flushes all the buffers to disk and fsyncs it.
func (w *Writer) Flush() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flush error: while syncing: %w", err)
	}

	return nil
}

// Close closes the file.
func (w *Writer) Close() error {
	err := w.file.Close()
	if err != nil {
		return fmt.Errorf("while closing the wal writer: %w", err)
	}

	return nil
}

// WriteBlock writes the WAL block to storage.
func (w *Writer) WriteBlock(data []byte) error {
	const walBlockSize = 1 << 20

	// Process data in blocks
	for start := 0; start < len(data); start += walBlockSize {
		end := start + walBlockSize
		if end > len(data) {
			end = len(data) // ensure we don't go out of bounds
		}
		block := data[start:end]

		if err := w.writeBlockInternal(block); err != nil {
			return fmt.Errorf("while writing WAL block: %w", err)
		}
	}

	return nil
}

// WriteBlock writes the WAL block to storage.
func (w *Writer) writeBlockInternal(p []byte) error {
	// Step 1: compression and encryption
	wrappedBlock, err := w.conn.WrapBlock(p)
	if err != nil {
		return fmt.Errorf("while wrapping WAL block: %w", err)
	}

	// Step 2: writing to permanent storage
	prefix := protowire.AppendFixed64(nil, uint64(len(wrappedBlock)))
	_, err = w.file.Write(prefix)
	if err != nil {
		return fmt.Errorf("while writing prefix: %w", err)
	}

	_, err = w.file.Write(wrappedBlock)
	if err != nil {
		return fmt.Errorf("while writing WAL file block: %w", err)
	}

	return nil
}

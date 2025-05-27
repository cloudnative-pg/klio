package walserver

import (
	"encoding/binary"
	"fmt"
	"os"
	"path"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

// WALWriter is the WAL file writer.
type WALWriter struct {
	walFilePath        string
	walFilePartialPath string
	conn               *repository.Connection

	file *os.File
}

// NewWALWriter creates a new WAL file writer.
func NewWALWriter(conn *repository.Connection, clusterName, walName string) (*WALWriter, error) {
	if err := validateWalFileName(walName); err != nil {
		return nil, err
	}

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

	return &WALWriter{
		walFilePath:        walFilePath,
		walFilePartialPath: walFilePartialPath,
		file:               file,
		conn:               conn,
	}, nil
}

// CloseMarkDone closes the WAL writer and mark the file as completed.
func (w *WALWriter) CloseMarkDone() error {
	if err := w.Close(); err != nil {
		return fmt.Errorf("while closing partial file: %w", err)
	}

	if err := os.Rename(w.walFilePartialPath, w.walFilePath); err != nil {
		return fmt.Errorf("while renaming partial file: %w", err)
	}

	return nil
}

// Flush flushes all the buffers to disk and fsyncs it.
func (w *WALWriter) Flush() error {
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flush error: while syncing: %w", err)
	}

	return nil
}

// Close closes the file.
func (w *WALWriter) Close() error {
	return w.file.Close()
}

// WriteBlock writes the WAL block to storage
func (w *WALWriter) WriteBlock(p []byte) error {
	if len(p) > int(maxBlockLen) {
		return ErrWrongBlockLen
	}

	// Step 1: compression and encryption
	wrappedBlock, err := w.conn.WrapBlock(p)
	if err != nil {
		return err
	}

	// Step 2: writing to permanent storage
	blockLen := make([]byte, 8)
	binary.BigEndian.PutUint64(blockLen, uint64(len(wrappedBlock)))

	if _, err := w.file.Write(blockLen); err != nil {
		return err
	}

	_, err = w.file.Write(wrappedBlock)
	return err
}

package klioserver

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
)

// WALWriter is the WAL file writer.
type WALWriter struct {
	walFilePath        string
	walFilePartialPath string

	file             *os.File
	encryptingWriter io.WriteCloser
	gzipWriter       *gzip.Writer
}

// NewWALWriter creates a new WAL file writer.
func NewWALWriter(conn *repository.Connection, clusterName, walName string) (*WALWriter, error) {
	if len(walName) < 16 {
		return nil, NewIncorrectWALNameError(walName)
	}

	walFileBase := path.Join(conn.BaseDir(), clusterName, walName[0:16])
	walFilePartialPath := path.Join(walFileBase, walName+".partial")
	walFilePath := path.Join(walFileBase, walName)

	// Step 1: ensure the parent path exists
	err := os.MkdirAll(walFileBase, 0o750)
	if err != nil {
		return nil, fmt.Errorf(
			"error while creating directory %s: %w",
			walFileBase,
			err,
		)
	}

	// Step 2: open the file
	//nolint:gosec
	file, err := os.OpenFile(walFilePartialPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			walFilePath,
			err,
		)
	}

	encryptingWriter, err := conn.ProtectWriter(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("while creating encrypted writer: %w", err)
	}

	gzipWriter := gzip.NewWriter(encryptingWriter)

	return &WALWriter{
		walFilePath:        walFilePath,
		walFilePartialPath: walFilePartialPath,
		file:               file,
		gzipWriter:         gzipWriter,
		encryptingWriter:   encryptingWriter,
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
	if err := w.gzipWriter.Flush(); err != nil {
		return fmt.Errorf("flush error: while flushing: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flush error: while synching: %w", err)
	}

	return nil
}

// Close closes the file.
func (w *WALWriter) Close() error {
	if err := w.gzipWriter.Close(); err != nil {
		return fmt.Errorf("close error: while closing gzipwriter: %w", err)
	}

	if err := w.encryptingWriter.Close(); err != nil {
		return fmt.Errorf("close error: while closing encrypting writer: %w", err)
	}

	// the encrypting writer is closing the underlying file

	return nil
}

// Write writes data.
func (w *WALWriter) Write(p []byte) (int, error) {
	return w.gzipWriter.Write(p) //nolint:wrapcheck
}

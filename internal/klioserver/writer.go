package klioserver

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
)

// Flusher is implemented by writers allowing to flush blocks.
type Flusher interface {
	Flush() error
}

// WALWriter is the WAL file writer.
type WALWriter struct {
	io.WriteCloser
	Flusher

	walFilePath        string
	walFilePartialPath string
}

// NewWALWriter creates a new WAL file writer.
func NewWALWriter(baseDir, clusterName, walName string) (*WALWriter, error) {
	if len(walName) < 16 {
		return nil, NewIncorrectWALNameError(walName)
	}

	walFileBase := path.Join(baseDir, clusterName, walName[0:16])
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
	walFileWriter, err := os.OpenFile(walFilePartialPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			walFilePath,
			err,
		)
	}

	gzipWriter := gzip.NewWriter(walFileWriter)

	return &WALWriter{
		walFilePath:        walFilePath,
		walFilePartialPath: walFilePartialPath,
		WriteCloser:        gzipWriter,
		Flusher:            gzipWriter,
	}, nil
}

// CloseMarkDone closes the WAL writer and mark the file as completed.
func (w *WALWriter) CloseMarkDone() error {
	err := w.WriteCloser.Close()
	if err != nil {
		return fmt.Errorf("while closing partial file: %w", err)
	}

	err = os.Rename(w.walFilePartialPath, w.walFilePath)
	if err != nil {
		return fmt.Errorf("while renaming partial file: %w", err)
	}

	return nil
}

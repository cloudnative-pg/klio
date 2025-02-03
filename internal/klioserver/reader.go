package klioserver

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
)

// WALReader is the WAL file writer.
type WALReader struct {
	io.ReadCloser

	walFilePath string
}

// NewWALReader creates a new WAL file writer.
func NewWALReader(baseDir, clusterName, walName string) (*WALReader, error) {
	if len(walName) < 16 {
		return nil, NewIncorrectWALNameError(walName)
	}

	walFileBase := path.Join(baseDir, clusterName, walName[0:16])
	walFilePath := path.Join(walFileBase, walName)

	//nolint:gosec
	walFileReader, err := os.OpenFile(walFilePath, os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			walFilePath,
			err,
		)
	}

	gzipReader, err := gzip.NewReader(walFileReader)
	if err != nil {
		return nil, fmt.Errorf("while reading WAL (gzip): %w", err)
	}

	return &WALReader{
		walFilePath: walFilePath,
		ReadCloser:  gzipReader,
	}, nil
}

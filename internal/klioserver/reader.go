package klioserver

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
)

// WALReader is the WAL file writer.
type WALReader struct {
	io.ReadCloser

	walFilePath string
}

// NewWALReader creates a new WAL file writer.
func NewWALReader(conn *repository.Connection, clusterName, walName string) (*WALReader, error) {
	const expectedWalFileNameLength = 16
	if len(walName) < expectedWalFileNameLength {
		return nil, NewIncorrectWALNameError(walName)
	}

	walFileBase := path.Join(conn.BaseDir(), clusterName, walName[0:expectedWalFileNameLength])
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

	decryptingReader, err := conn.ProtectReader(walFileReader)
	if err != nil {
		_ = walFileReader.Close()
		return nil, fmt.Errorf(
			"error while creating decrypting reader for file %s: %w",
			walFilePath,
			err,
		)
	}

	gzipReader, err := gzip.NewReader(decryptingReader)
	if err != nil {
		return nil, fmt.Errorf("while reading WAL (gzip): %w", err)
	}

	return &WALReader{
		walFilePath: walFilePath,
		ReadCloser:  gzipReader,
	}, nil
}

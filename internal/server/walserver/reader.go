package walserver

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

// ErrWrongBlockLen is raised when a block
var ErrWrongBlockLen = fmt.Errorf("incorrect wrong WAL block length")

// maxBlockLen is the maximum size of a written block
const maxBlockLen = uint64(32 * 1024 * 1024)

// WALReader is the WAL file writer.
type WALReader struct {
	conn        *repository.Connection
	file        *os.File
	walFilePath string
}

// NewWALReader creates a new WAL file writer.
func NewWALReader(
	conn *repository.Connection,
	clusterName,
	walName string,
) (*WALReader, error) {
	if err := validateWalFileName(walName); err != nil {
		return nil, err
	}

	walFilePath := getWALArchivePath(conn.BaseDir(), clusterName, walName)

	//nolint:gosec
	walFileReader, err := os.OpenFile(walFilePath, os.O_RDONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			walFilePath,
			err,
		)
	}

	return &WALReader{
		conn:        conn,
		walFilePath: walFilePath,
		file:        walFileReader,
	}, nil
}

// Close closes the file
func (r *WALReader) Close() error {
	return r.file.Close()
}

// ReadBlock reads the next WAL block from the file
func (r *WALReader) ReadBlock() ([]byte, error) {
	// Step 1: read length of the next block
	blockLen := make([]byte, 8)
	if _, err := r.file.Read(blockLen); err != nil {
		return nil, err
	}

	// Step 2: read block
	blockLenDecoded := binary.BigEndian.Uint64(blockLen)
	if blockLenDecoded >= maxBlockLen {
		return nil, ErrWrongBlockLen
	}

	block := make([]byte, blockLenDecoded)
	if _, err := r.file.Read(block); err != nil {
		return nil, err
	}

	return r.conn.UnwrapBlock(block)
}

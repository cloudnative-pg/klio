package walserver

import (
	"bufio"
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

// WALReader is the WAL file writer.
type WALReader struct {
	conn        *repository.Connection
	file        *os.File
	reader      *bufio.Reader
	walFilePath string

	segmentLength uint64
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

	reader := bufio.NewReader(walFileReader)
	header := grpc.StartWALFile{}

	if err := protodelim.UnmarshalFrom(reader, &header); err != nil {
		return nil, fmt.Errorf("while reading WAL file header: %w", err)
	}

	return &WALReader{
		conn:          conn,
		walFilePath:   walFilePath,
		file:          walFileReader,
		reader:        reader,
		segmentLength: header.GetFileLength(),
	}, nil
}

// Close closes the file.
func (r *WALReader) Close() error {
	err := r.file.Close()
	if err != nil {
		return fmt.Errorf("while closing the walreader: %w", err)
	}

	return nil
}

// ReadBlock reads the next WAL block from the file.
func (r *WALReader) ReadBlock() ([]byte, error) {
	block := grpc.WALFileBlock{}
	if err := protodelim.UnmarshalFrom(r.reader, &block); err != nil {
		return nil, fmt.Errorf("while reading WAL file block: %w", err)
	}

	bytesRead, err := r.conn.UnwrapBlock(block.GetRange())
	if err != nil {
		return nil, fmt.Errorf("while unwrapping WAL file block: %w", err)
	}

	return bytesRead, nil
}

// GetFileLength gets the file length.
func (r *WALReader) GetFileLength() uint64 {
	return r.segmentLength
}

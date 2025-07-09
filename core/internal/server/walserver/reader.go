package walserver

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
)

// Reader is the WAL file writer.
type Reader struct {
	conn        *repository.Connection
	file        *os.File
	reader      *bufio.Reader
	walFilePath string

	segmentLength uint64
}

// NewReader creates a new WAL file writer.
func NewReader(
	conn *repository.Connection,
	clusterName,
	walName string,
) (*Reader, error) {
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

	return &Reader{
		conn:          conn,
		walFilePath:   walFilePath,
		file:          walFileReader,
		reader:        reader,
		segmentLength: header.GetFileLength(),
	}, nil
}

// Close closes the file.
func (r *Reader) Close() error {
	err := r.file.Close()
	if err != nil {
		return fmt.Errorf("while closing the walreader: %w", err)
	}

	return nil
}

// ReadBlock reads the next WAL block from the file.
func (r *Reader) ReadBlock() ([]byte, error) {
	blockLenBytes := make([]byte, 8)
	if _, err := io.ReadFull(r.reader, blockLenBytes); err != nil {
		return nil, fmt.Errorf("while reading WAL file block prefix: %w", err)
	}

	blockLen, _ := protowire.ConsumeFixed64(blockLenBytes)

	block := make([]byte, blockLen)
	if _, err := io.ReadFull(r.reader, block); err != nil {
		return nil, fmt.Errorf("while reading WAL file block: %w", err)
	}

	bytesRead, err := r.conn.UnwrapBlock(block)
	if err != nil {
		return nil, fmt.Errorf("while unwrapping WAL file block: %w", err)
	}

	return bytesRead, nil
}

// GetFileLength gets the file length.
func (r *Reader) GetFileLength() uint64 {
	return r.segmentLength
}

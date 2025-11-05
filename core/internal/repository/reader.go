package repository

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/afero"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// Reader is the WAL file writer.
type Reader struct {
	conn        *Connection
	file        afero.File
	reader      *bufio.Reader
	walFilePath string
	tracer      trace.Tracer

	segmentLength uint64
}

// NewReader creates a new WAL file writer.
func NewReader(
	conn *Connection,
	clusterName,
	walName string,
	tracer trace.Tracer,
) (*Reader, error) {
	walFilePath := getWALArchivePath(clusterName, walName)

	walFileReader, err := conn.fs.OpenFile(walFilePath, os.O_RDONLY, 0o600)
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
		tracer:        tracer,
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
func (r *Reader) ReadBlock(ctx context.Context) ([]byte, error) {
	block, err := r.readBlockData(ctx)
	if err != nil {
		return nil, err
	}

	return r.unwrapBlockData(ctx, block)
}

// GetFileLength gets the file length.
func (r *Reader) GetFileLength() uint64 {
	return r.segmentLength
}

// readBlockData reads the block length and data from the file.
func (r *Reader) readBlockData(ctx context.Context) ([]byte, error) {
	_, readBlockSpan := r.tracer.Start(ctx, opentelemetry.ReadBlockDataSpan)
	defer readBlockSpan.End()

	blockLenBytes := make([]byte, 8)
	_, err := io.ReadFull(r.reader, blockLenBytes)
	if err != nil {
		readBlockSpan.RecordError(fmt.Errorf("error while reading block: %w", err))
		return nil, fmt.Errorf("while reading WAL file block prefix: %w", err)
	}

	blockLen, _ := protowire.ConsumeFixed64(blockLenBytes)

	block := make([]byte, blockLen)
	if _, err := io.ReadFull(r.reader, block); err != nil {
		readBlockSpan.RecordError(fmt.Errorf("error while reading block: %w", err))
		return nil, fmt.Errorf("while reading WAL file block: %w", err)
	}

	return block, nil
}

// unwrapBlockData unwraps the block data using the connection.
func (r *Reader) unwrapBlockData(ctx context.Context, block []byte) ([]byte, error) {
	_, unwrapSpan := r.tracer.Start(ctx, opentelemetry.UnwrapBlockSpan)
	defer unwrapSpan.End()

	bytesRead, err := r.conn.UnwrapBlock(block)
	if err != nil {
		unwrapSpan.SetStatus(codes.Error, err.Error())
		unwrapSpan.RecordError(fmt.Errorf("error while unwrapping block: %w", err))
		return nil, fmt.Errorf("while unwrapping WAL file block: %w", err)
	}

	return bytesRead, nil
}

package repository

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/afero"
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
	clusterName string
	metrics     *Metrics

	segmentLength uint64
}

// NewReader creates a new WAL file reader. metrics may be nil to skip per-block
// duration recording (e.g. when reading non-WAL data such as cluster metadata).
func NewReader(
	conn *Connection,
	clusterName,
	walName string,
	metrics *Metrics,
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
		clusterName:   clusterName,
		file:          walFileReader,
		reader:        reader,
		metrics:       metrics,
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

// ReadBlock reads the next WAL block from the file, recording the per-block
// read and unwrap durations on the WAL duration histogram.
func (r *Reader) ReadBlock(ctx context.Context) ([]byte, error) {
	readStart := time.Now()
	block, err := r.readBlockData()
	readDuration := time.Since(readStart)
	if err != nil {
		// io.EOF is the normal end of the file, not a failure.
		if !errors.Is(err, io.EOF) {
			r.recordStage(ctx, opentelemetry.StageRead, readDuration, opentelemetry.OutcomeFailure)
		}

		return nil, err
	}
	r.recordStage(ctx, opentelemetry.StageRead, readDuration, opentelemetry.OutcomeSuccess)

	unwrapStart := time.Now()
	unwrapped, err := r.unwrapBlockData(block)
	unwrapDuration := time.Since(unwrapStart)
	if err != nil {
		r.recordStage(ctx, opentelemetry.StageUnwrap, unwrapDuration, opentelemetry.OutcomeFailure)

		return nil, err
	}
	r.recordStage(ctx, opentelemetry.StageUnwrap, unwrapDuration, opentelemetry.OutcomeSuccess)

	return unwrapped, nil
}

// GetFileLength gets the file length.
func (r *Reader) GetFileLength() uint64 {
	return r.segmentLength
}

// recordStage records a per-block read-side stage duration, if metrics are set.
func (r *Reader) recordStage(
	ctx context.Context,
	stage opentelemetry.Stage,
	d time.Duration,
	outcome opentelemetry.Outcome,
) {
	if r.metrics == nil {
		return
	}

	r.metrics.RecordBlockStage(ctx, r.clusterName, opentelemetry.PathGet, stage, d, outcome)
}

// readBlockData reads the block length and data from the file.
func (r *Reader) readBlockData() ([]byte, error) {
	blockLenBytes := make([]byte, 8)
	_, err := io.ReadFull(r.reader, blockLenBytes)
	if err != nil {
		return nil, fmt.Errorf("while reading WAL file block prefix: %w", err)
	}

	blockLen, _ := protowire.ConsumeFixed64(blockLenBytes)

	block := make([]byte, blockLen)
	if _, err := io.ReadFull(r.reader, block); err != nil {
		return nil, fmt.Errorf("while reading WAL file block: %w", err)
	}

	return block, nil
}

// unwrapBlockData unwraps the block data using the connection.
func (r *Reader) unwrapBlockData(block []byte) ([]byte, error) {
	bytesRead, err := r.conn.UnwrapBlock(block)
	if err != nil {
		return nil, fmt.Errorf("while unwrapping WAL file block: %w", err)
	}

	return bytesRead, nil
}

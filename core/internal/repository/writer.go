package repository

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/spf13/afero"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// Writer writes a WAL file atomically: data is written to a `.partial`
// file and renamed to the final path on CloseMarkDone.
type Writer struct {
	walFilePath        string
	walFilePartialPath string
	conn               *Connection
	inner              *DirectWriter
}

// WriterOptions configures a Writer.
type WriterOptions struct {
	// ClusterName is the name of the cluster the WAL file belongs to.
	ClusterName string

	// WALName is the name of the WAL segment being written.
	WALName string

	// SegmentSize is the length, in bytes, of the WAL segment.
	SegmentSize uint64

	// Metrics collects per-write metrics for this Writer.
	Metrics *Metrics

	// BufferSize is the size, in bytes, of the buffer placed in front of
	// the underlying file. When zero, defaultChunkSize is used.
	BufferSize int
}

// DirectWriter writes the Klio WAL file format (header + length-prefixed,
// compressed/encrypted blocks) directly to a WAL archive file, without
// the `.partial` + rename dance performed by Writer.
type DirectWriter struct {
	conn        *Connection
	clusterName string
	metrics     *Metrics

	file   afero.File
	buffer *bufio.Writer
}

// NewWriter creates a new WAL file writer.
func (c *Connection) NewWriter(opts WriterOptions) (*Writer, error) {
	walFilePath := getWALArchivePath(opts.ClusterName, opts.WALName)
	walFilePartialPath := walFilePath + ".partial"

	inner, err := c.newDirectWriterAtPath(walFilePartialPath, opts)
	if err != nil {
		return nil, err
	}

	return &Writer{
		walFilePath:        walFilePath,
		walFilePartialPath: walFilePartialPath,
		conn:               c,
		inner:              inner,
	}, nil
}

// NewDirectWriter creates a non-atomic WAL writer that writes directly
// to the final WAL archive path. Use NewWriter when atomic semantics
// (write to `.partial`, rename on success) are required.
func (c *Connection) NewDirectWriter(opts WriterOptions) (*DirectWriter, error) {
	return c.newDirectWriterAtPath(
		getWALArchivePath(opts.ClusterName, opts.WALName),
		opts,
	)
}

// newDirectWriterAtPath opens the WAL archive file at the given path,
// creating any missing parent directories, and wraps it in a DirectWriter.
func (c *Connection) newDirectWriterAtPath(filePath string, opts WriterOptions) (*DirectWriter, error) {
	if err := c.fs.MkdirAll(path.Dir(filePath), 0o750); err != nil {
		return nil, fmt.Errorf(
			"error while creating directory %s: %w",
			path.Base(filePath),
			err,
		)
	}

	file, err := c.fs.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf(
			"error while opening file %s: %w",
			filePath,
			err,
		)
	}

	inner, err := c.newDirectWriter(file, opts)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	return inner, nil
}

// CloseMarkDone closes the WAL writer and marks the file as completed
// by renaming the partial file to its final path.
func (w *Writer) CloseMarkDone() error {
	if err := w.inner.Close(); err != nil {
		return fmt.Errorf("while closing partial file: %w", err)
	}

	if err := w.conn.fs.Rename(w.walFilePartialPath, w.walFilePath); err != nil {
		return fmt.Errorf("while renaming partial file: %w", err)
	}

	return nil
}

// Flush flushes all the buffers to disk and fsyncs the underlying file.
func (w *Writer) Flush() error {
	return w.inner.Flush()
}

// Close closes the writer without renaming the partial file.
func (w *Writer) Close() error {
	return w.inner.Close()
}

// WriteBlock writes the WAL block to storage.
func (w *Writer) WriteBlock(ctx context.Context, data []byte) error {
	return w.inner.WriteBlock(ctx, data)
}

// newDirectWriter creates a format writer over an already-open file and
// writes the Klio header.
func (c *Connection) newDirectWriter(
	file afero.File,
	opts WriterOptions,
) (*DirectWriter, error) {
	startBlock := grpc.StartWALFile{
		KlioVersion: 1,
		FileLength:  opts.SegmentSize,
	}

	bufferSize := opts.BufferSize
	if bufferSize == 0 {
		bufferSize = defaultChunkSize
	}
	buffer := bufio.NewWriterSize(file, bufferSize)
	if _, err := protodelim.MarshalTo(buffer, &startBlock); err != nil {
		return nil, fmt.Errorf("while writing WAL file header: %w", err)
	}

	return &DirectWriter{
		conn:        c,
		clusterName: opts.ClusterName,
		metrics:     opts.Metrics,
		file:        file,
		buffer:      buffer,
	}, nil
}

// Flush flushes all the buffers to disk and fsyncs it.
func (w *DirectWriter) Flush() error {
	if err := w.buffer.Flush(); err != nil {
		return fmt.Errorf("flush: while writing buffer: %w", err)
	}

	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flush: while syncing: %w", err)
	}

	return nil
}

// Close flushes the buffer and closes the underlying file.
func (w *DirectWriter) Close() error {
	if err := w.buffer.Flush(); err != nil {
		return fmt.Errorf("close: while writing buffer: %w", err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("while closing the wal writer: %w", err)
	}

	return nil
}

// WriteBlock writes the WAL block to storage. The per-chunk wrap and write
// durations are accumulated and emitted as one observation per stage for the
// whole block, so they share the per-block granularity of the flush stage (one
// fsync per block).
func (w *DirectWriter) WriteBlock(ctx context.Context, data []byte) error {
	const walBlockSize = 1 << 20

	// Nothing to wrap or write for an empty block: skip it without emitting a
	// zero-duration observation.
	if len(data) == 0 {
		return nil
	}

	var wrap, write time.Duration
	var chunkWritten bool
	for start := 0; start < len(data); start += walBlockSize {
		end := start + walBlockSize
		end = min(end, len(data))

		// Step 1: compression and encryption
		wrapStart := time.Now()
		wrappedBlock, err := w.wrapBlock(data[start:end])
		wrap += time.Since(wrapStart)
		if err != nil {
			w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
				opentelemetry.StageWrap, wrap, opentelemetry.OutcomeFailure)
			// A prior chunk in this block may have already completed its write
			// step; that work happened and must not be dropped just because a
			// later chunk failed to wrap.
			if chunkWritten {
				w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
					opentelemetry.StageWrite, write, opentelemetry.OutcomeSuccess)
			}

			return fmt.Errorf("while wrapping WAL block: %w", err)
		}

		// Step 2: writing to permanent storage
		writeStart := time.Now()
		err = w.writeBlockData(ctx, wrappedBlock)
		write += time.Since(writeStart)
		if err != nil {
			w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
				opentelemetry.StageWrap, wrap, opentelemetry.OutcomeSuccess)
			w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
				opentelemetry.StageWrite, write, opentelemetry.OutcomeFailure)

			return fmt.Errorf("while writing WAL block data: %w", err)
		}
		chunkWritten = true
	}

	w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
		opentelemetry.StageWrap, wrap, opentelemetry.OutcomeSuccess)
	w.metrics.RecordBlockStage(ctx, w.clusterName, opentelemetry.PathPut,
		opentelemetry.StageWrite, write, opentelemetry.OutcomeSuccess)

	return nil
}

// wrapBlock compresses and encrypts the block data.
func (w *DirectWriter) wrapBlock(p []byte) ([]byte, error) {
	wrappedBlock, err := w.conn.WrapBlock(p, defaultChunkSize)
	if err != nil {
		return nil, fmt.Errorf("error while wrapping block: %w", err)
	}

	return wrappedBlock, nil
}

// writeBlockData writes the wrapped block data to the buffer and updates the
// bytes-written counter.
func (w *DirectWriter) writeBlockData(ctx context.Context, wrappedBlock []byte) error {
	prefix := protowire.AppendFixed64(nil, uint64(len(wrappedBlock)))
	nBytes, err := w.buffer.Write(prefix)
	if err != nil {
		return fmt.Errorf("while writing prefix: %w", err)
	}

	w.metrics.WalWrittenBytes.Add(ctx, int64(nBytes),
		metric.WithAttributeSet(
			w.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(w.clusterName)),
		),
	)

	nBytes, err = w.buffer.Write(wrappedBlock)
	if err != nil {
		return fmt.Errorf("while writing WAL file block: %w", err)
	}
	w.metrics.WalWrittenBytes.Add(ctx, int64(nBytes),
		metric.WithAttributeSet(
			w.metrics.AttributeSet(opentelemetry.AttributeKeyClusterName.Of(w.clusterName)),
		),
	)

	return nil
}

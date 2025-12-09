package repository

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// NewDummyMetrics creates a dummy metrics instance for testing purposes.
func NewDummyMetrics() *Metrics {
	meter := otel.Meter(opentelemetry.Meter)

	walWrittenBytes, _ := meter.Int64Counter(
		"dummy.wal.written_size",
		metric.WithDescription("Number of bytes written to disk for the WAL files"),
		metric.WithUnit("By"),
	)
	walWritten, _ := meter.Int64Counter(
		"dummy.wal.written",
		metric.WithDescription("Number of WAL files written"),
		metric.WithUnit("{wals}"),
	)

	return &Metrics{
		WalWrittenBytes: walWrittenBytes,
		WalWritten:      walWritten,
	}
}

var dummyTracer = otel.Tracer("dummy") //nolint:gochecknoglobals

func TestWALReaderBlockSplit(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	defer conn.Close()

	readerNonExisting, err := NewReader(conn, "cluster-example", "0000001000000000000001FF", dummyTracer)
	assert.Nil(t, readerNonExisting)
	require.ErrorIs(t, err, os.ErrNotExist)

	const fileLen = uint64(16 * 1024 * 1024)
	metrics := NewDummyMetrics()
	writer, err := conn.NewWriter("cluster-example", "0000001000000000000001FF", fileLen, metrics, dummyTracer)
	require.NoError(t, err)
	assert.NotNil(t, writer)

	buffer := make([]byte, fileLen)
	_, _ = rand.Read(buffer)

	err = writer.WriteBlock(t.Context(), buffer)
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001FF", dummyTracer)
	require.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, fileLen, reader.GetFileLength())

	// Read the splitted blocks
	var wBlocks bytes.Buffer
	for {
		innerBlock, err := reader.ReadBlock(t.Context())
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		_, _ = wBlocks.Write(innerBlock)
	}
	require.Equal(t, len(buffer), wBlocks.Len())
	require.Equal(t, buffer, wBlocks.Bytes())

	err = reader.Close()
	require.NoError(t, err)
}

func TestReaderWriterBlocks(t *testing.T) {
	// Step 1: write two blocks to the file
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	defer conn.Close()

	const fileLen = uint64(145)
	metrics := NewDummyMetrics()
	writer, err := conn.NewWriter("cluster-example", "0000001000000000000001F8", fileLen, metrics, dummyTracer)
	require.NoError(t, err)
	require.NotNil(t, writer)

	block1 := []byte("this-test")
	err = writer.WriteBlock(t.Context(), block1)
	require.NoError(t, err)

	block2 := []byte("toast-is-good")
	err = writer.WriteBlock(t.Context(), block2)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	// Step 2: open the compressed file
	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001F8", dummyTracer)
	require.NoError(t, err)
	assert.NotNil(t, reader)
	assert.Equal(t, fileLen, reader.GetFileLength())

	// Step 2.1: read the first block
	block1Read, err := reader.ReadBlock(t.Context())
	require.NoError(t, err)
	assert.Equal(t, block1, block1Read)

	// Step 2.2: read the second block
	block2Read, err := reader.ReadBlock(t.Context())
	require.NoError(t, err)
	assert.Equal(t, block2, block2Read)

	err = reader.Close()
	require.NoError(t, err)
}

func TestReaderWriter100KBlocks(t *testing.T) {
	// Step 1: write two blocks to the file
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	defer conn.Close()

	block1 := make([]byte, 131072)
	_, _ = rand.Read(block1)

	fileLen := uint64(128 * len(block1))
	metrics := NewDummyMetrics()
	writer, err := conn.NewWriter("cluster-example", "0000001000000000000001F8", fileLen, metrics, dummyTracer)
	require.NoError(t, err)
	require.NotNil(t, writer)

	for range 128 {
		err = writer.WriteBlock(t.Context(), block1)
		require.NoError(t, err)

		err = writer.Flush()
		require.NoError(t, err)
	}

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)
}

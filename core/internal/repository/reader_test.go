package repository

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

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
	latestWrittenTime, _ := meter.Int64Gauge(
		"dummy.wal.latest_written_time",
		metric.WithDescription("Latest time a WAL file was written to disk"),
		metric.WithUnit("s"),
	)

	return &Metrics{
		WalWrittenBytes:   walWrittenBytes,
		WalWritten:        walWritten,
		LatestWrittenTime: latestWrittenTime,
	}
}

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

	readerNonExisting, err := NewReader(conn, "cluster-example", "0000001000000000000001FF", nil)
	assert.Nil(t, readerNonExisting)
	require.ErrorIs(t, err, os.ErrNotExist)

	const fileLen = uint64(16 * 1024 * 1024)
	metrics := NewDummyMetrics()
	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001FF",
			SegmentSize: fileLen,
			Metrics:     metrics,
		},
	)
	require.NoError(t, err)
	assert.NotNil(t, writer)

	buffer := make([]byte, fileLen)
	_, _ = rand.Read(buffer)

	err = writer.WriteBlock(t.Context(), buffer)
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001FF", metrics)
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
	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001F8",
			SegmentSize: fileLen,
			Metrics:     metrics,
		},
	)
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
	reader, err := NewReader(conn, "cluster-example", "0000001000000000000001F8", metrics)
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

	fileLen, err := safecast.Convert[uint64](128 * len(block1))
	require.NoError(t, err)

	metrics := NewDummyMetrics()
	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001F8",
			SegmentSize: fileLen,
			Metrics:     metrics,
		},
	)
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

// TestReaderRecordsReadAndUnwrapStages verifies that ReadBlock records the
// read-side per-block stages (read, unwrap) on the BlockDuration histogram,
// carrying the tier attribute baked into the Metrics.
func TestReaderRecordsReadAndUnwrapStages(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}
	require.NoError(t, Initialize(opts))

	conn, err := Open(opts)
	require.NoError(t, err)
	require.NotNil(t, conn)

	defer conn.Close()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	blockDuration, err := provider.Meter("test").Int64Histogram(opentelemetry.ServerWalBlockDurationMetric)
	require.NoError(t, err)

	metrics := NewDummyMetrics()
	metrics.BlockDuration = blockDuration
	metrics.Attributes = []attribute.KeyValue{opentelemetry.Tier1.Attribute()}

	const fileLen = uint64(64)
	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001F8",
			SegmentSize: fileLen,
			Metrics:     metrics,
		},
	)
	require.NoError(t, err)
	require.NoError(t, writer.WriteBlock(t.Context(), []byte("hello-wal")))
	require.NoError(t, writer.Flush())
	require.NoError(t, writer.CloseMarkDone())

	walReader, err := NewReader(conn, "cluster-example", "0000001000000000000001F8", metrics)
	require.NoError(t, err)
	for {
		_, readErr := walReader.ReadBlock(t.Context())
		if errors.Is(readErr, io.EOF) {
			break
		}
		require.NoError(t, readErr)
	}

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	stagePath := map[string]string{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ServerWalBlockDurationMetric {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[int64])
			require.True(t, ok)
			for _, dp := range h.DataPoints {
				stage, ok := dp.Attributes.Value(attribute.Key("stage"))
				require.True(t, ok, "every data point must carry a stage attribute")
				pathVal, ok := dp.Attributes.Value(attribute.Key("path"))
				require.True(t, ok, "every data point must carry a path attribute")
				tier, ok := dp.Attributes.Value(attribute.Key("tier"))
				require.True(t, ok, "every data point must carry a tier attribute")
				assert.Equal(t, string(opentelemetry.Tier1), tier.AsString())
				stagePath[stage.AsString()] = pathVal.AsString()
			}
		}
	}

	// The read-side stages must be recorded on the get path.
	assert.Equal(t, string(opentelemetry.PathGet), stagePath[string(opentelemetry.StageRead)],
		"read stage must be recorded on the get path")
	assert.Equal(t, string(opentelemetry.PathGet), stagePath[string(opentelemetry.StageUnwrap)],
		"unwrap stage must be recorded on the get path")
}

// TestNewReaderEmptyFileIsNotFound verifies that a WAL file which exists but has
// no readable header (zero bytes) is reported as os.ErrNotExist. On an S3
// backend a missing segment whose name is a prefix of an existing .partial
// object surfaces this way, and callers must treat it as missing so they fall
// back to the .partial variant instead of failing hard.
func TestNewReaderEmptyFileIsNotFound(t *testing.T) {
	opts := Options{FS: afero.NewMemMapFs(), Password: "this-password"}
	require.NoError(t, Initialize(opts))

	conn, err := Open(opts)
	require.NoError(t, err)
	defer conn.Close()

	const (
		clusterName = "cluster-example"
		walName     = "000000010000000000000001"
	)

	// An empty file at the WAL archive path mimics a zero-byte object.
	require.NoError(t, afero.WriteFile(conn.fs, getWALArchivePath(clusterName, walName), nil, 0o600))

	_, err = NewReader(conn, clusterName, walName, NewDummyMetrics())
	require.ErrorIs(t, err, os.ErrNotExist)
}

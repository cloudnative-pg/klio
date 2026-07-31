/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package repository

import (
	"context"
	"crypto/rand"
	"path"
	"testing"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/wal"
)

func TestWriter(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	const fileLen = 123
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

	block := []byte("this-test")
	err = writer.WriteBlock(t.Context(), block)
	require.NoError(t, err)

	err = writer.Flush()
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	expectedPath := path.Join("cluster-example", "0000001000000000", "0000001000000000000001F8")
	exists, err := FileExists(opts.FS, expectedPath)
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := opts.FS.Stat(expectedPath)
	require.NoError(t, err)
	assert.Greater(t, data.Size(), int64(len(block)))
}

// TestWriterWALFilePathAfterCloseMarkDone verifies that once a WAL segment is
// marked done, WALFilePath returns the final name without the .partial suffix.
func TestWriterWALFilePathAfterCloseMarkDone(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}
	require.NoError(t, Initialize(opts))

	conn, err := Open(opts)
	require.NoError(t, err)
	defer conn.Close()

	const fileLen = 123
	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001F8",
			SegmentSize: fileLen,
			Metrics:     NewDummyMetrics(),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, writer)
	assert.Contains(t, writer.WALFilePath(), wal.PartialSuffix)

	err = writer.WriteBlock(t.Context(), []byte("this-test"))
	require.NoError(t, err)

	err = writer.CloseMarkDone()
	require.NoError(t, err)

	expectedPath := path.Join("cluster-example", "0000001000000000", "0000001000000000000001F8")
	assert.Equal(t, expectedPath, writer.WALFilePath())
	assert.NotContains(t, writer.WALFilePath(), wal.PartialSuffix)
}

func TestDirectWriter(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	assert.NotNil(t, conn)
	require.NoError(t, err)

	const fileLen = 123
	metrics := NewDummyMetrics()
	writer, err := conn.NewDirectWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001F8",
			SegmentSize: fileLen,
			Metrics:     metrics,
			BufferSize:  4 * 1024 * 1024,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, writer)

	block := []byte("this-test")
	err = writer.WriteBlock(t.Context(), block)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	expectedPath := path.Join("cluster-example", "0000001000000000", "0000001000000000000001F8")
	exists, err := FileExists(opts.FS, expectedPath)
	require.NoError(t, err)
	assert.True(t, exists)

	data, err := opts.FS.Stat(expectedPath)
	require.NoError(t, err)
	assert.Greater(t, data.Size(), int64(len(block)))
}

// poisoningInt64Counter corrupts conn's master key once Add has been called
// poisonAfter times. It stands in for WalWrittenBytes, the only metric
// recorded synchronously between chunks inside WriteBlock's loop, so it doubles
// as the seam needed to make a later chunk's WrapBlock call fail (a bad key
// size fails sio.EncryptWriter immediately) while an earlier chunk in the same
// WriteBlock call has already completed its write.
type poisoningInt64Counter struct {
	noop.Int64Counter

	conn        *Connection
	poisonAfter int
	calls       int
}

func (c *poisoningInt64Counter) Add(context.Context, int64, ...metric.AddOption) {
	c.calls++
	if c.calls == c.poisonAfter {
		c.conn.masterKey = []byte("too-short-to-be-a-valid-key")
	}
}

// TestWriteBlockPreservesWriteDurationWhenLaterChunkFailsToWrap verifies that
// when a multi-chunk block's later chunk fails to wrap, the write-stage
// duration already accumulated for earlier, successfully-written chunks is
// still recorded, instead of being silently dropped.
func TestWriteBlockPreservesWriteDurationWhenLaterChunkFailsToWrap(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}
	require.NoError(t, Initialize(opts))

	conn, err := Open(opts)
	require.NoError(t, err)
	defer conn.Close()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	blockDuration, err := provider.Meter("test").Int64Histogram(opentelemetry.ServerWalBlockDurationMetric)
	require.NoError(t, err)

	metrics := NewDummyMetrics()
	metrics.BlockDuration = blockDuration
	metrics.Attributes = []attribute.KeyValue{opentelemetry.Tier1.Attribute()}
	// The first 1MB chunk performs 2 WalWrittenBytes.Add calls (length prefix,
	// then block data); poisoning the key right after those corrupts it before
	// the second chunk's WrapBlock call, once the first chunk's write is done.
	metrics.WalWrittenBytes = &poisoningInt64Counter{conn: conn, poisonAfter: 2}

	writer, err := conn.NewDirectWriter(WriterOptions{
		ClusterName: "cluster-example",
		WALName:     "0000001000000000000001F8",
		SegmentSize: 2 << 20,
		Metrics:     metrics,
	})
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()

	data := make([]byte, 2<<20) // 2MB: WriteBlock splits this into two 1MB chunks.
	_, err = rand.Read(data)
	require.NoError(t, err)

	err = writer.WriteBlock(t.Context(), data)
	require.Error(t, err, "the second chunk's wrap must fail once the master key is poisoned")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &rm))

	type stageOutcome struct {
		stage, outcome string
	}
	counts := map[stageOutcome]uint64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ServerWalBlockDurationMetric {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[int64])
			require.True(t, ok)
			for _, dp := range h.DataPoints {
				stage, _ := dp.Attributes.Value(attribute.Key("stage"))
				outcome, _ := dp.Attributes.Value(attribute.Key("outcome"))
				counts[stageOutcome{stage.AsString(), outcome.AsString()}] = dp.Count
			}
		}
	}

	assert.Equal(t, uint64(1), counts[stageOutcome{"wrap", "failure"}],
		"the failing chunk's wrap must be recorded as a failure")
	assert.Equal(t, uint64(1), counts[stageOutcome{"write", "success"}],
		"the first chunk's completed write must not be dropped just because a later chunk failed to wrap")
}

func BenchmarkWriter(b *testing.B) {
	block := make([]byte, 100*1024)
	_, _ = rand.Read(block)

	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}

	err := Initialize(opts)
	require.NoError(b, err)

	conn, err := Open(opts)
	assert.NotNil(b, conn)
	require.NoError(b, err)

	defer conn.Close()

	metrics := NewDummyMetrics()
	segmentSize, err := safecast.Convert[uint64](len(block) * b.N)
	require.NoError(b, err)

	writer, err := conn.NewWriter(
		WriterOptions{
			ClusterName: "cluster-example",
			WALName:     "0000001000000000000001FF",
			SegmentSize: segmentSize,
			Metrics:     metrics,
		},
	)
	require.NoError(b, err)
	assert.NotNil(b, writer)

	b.ResetTimer()
	b.SetBytes(int64(len(block) * b.N))
	for range b.N {
		err := writer.WriteBlock(b.Context(), block)
		require.NoError(b, err)

		err = writer.Flush()
		require.NoError(b, err)
	}

	err = writer.CloseMarkDone()
	require.NoError(b, err)
}

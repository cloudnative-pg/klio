package consumer

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// TestWalHandlerRecordsErrorOnTier2UploadSpan verifies that a failed tier-2
// upload marks the tier2_upload span as errored, instead of leaving it
// looking like a successful operation in traces.
func TestWalHandlerRecordsErrorOnTier2UploadSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	previousTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() { otel.SetTracerProvider(previousTracerProvider) })

	opts := repository.Options{
		FS:       afero.NewMemMapFs(),
		Password: "this-password",
	}
	require.NoError(t, repository.Initialize(opts))

	tier1, err := repository.Open(opts)
	require.NoError(t, err)
	defer tier1.Close()

	w := NewWAL(&WALOptions{Tier1: tier1, Tier2: tier1})

	// The requested WAL does not exist on tier-1, so the reader creation
	// fails and walHandler returns an error without ever reaching tier-2.
	task := &queue.WALTask{
		ClusterName: "cluster-example",
		WALName:     "0000000100000000000000FF",
	}
	handlerErr := w.walHandler(t.Context(), task)
	require.Error(t, handlerErr)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, opentelemetry.Tier2UploadSpan, spans[0].Name)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "tier-2 upload failed", spans[0].Status.Description)

	require.Len(t, spans[0].Events, 1, "span must carry a recorded-error event")
	assert.Equal(t, "exception", spans[0].Events[0].Name)
}

// TestWalHandlerArchivesPartialWALFile verifies that a WAL segment left on
// tier-1 under its partial name, as happens when a failover interrupts the
// Put stream before the segment is fully received, is archived to tier-2
// under that same partial name instead of being silently dropped because
// tier-2 was asked to look up the bare (non-partial) name.
func TestWalHandlerArchivesPartialWALFile(t *testing.T) {
	const (
		clusterName = "cluster-example"
		walName     = "000000010000000000000001.partial"
	)

	tier1Opts := repository.Options{FS: afero.NewMemMapFs(), Password: "tier1-password"}
	require.NoError(t, repository.Initialize(tier1Opts))
	tier1, err := repository.Open(tier1Opts)
	require.NoError(t, err)
	defer tier1.Close()

	tier2Opts := repository.Options{FS: afero.NewMemMapFs(), Password: "tier2-password"}
	require.NoError(t, repository.Initialize(tier2Opts))
	tier2, err := repository.Open(tier2Opts)
	require.NoError(t, err)
	defer tier2.Close()

	w := NewWAL(&WALOptions{Tier1: tier1, Tier2: tier2})

	writer, err := tier1.NewWriter(repository.WriterOptions{
		ClusterName: clusterName,
		WALName:     "000000010000000000000001",
		SegmentSize: 16,
		Metrics:     w.metrics,
	})
	require.NoError(t, err)
	require.NoError(t, writer.WriteBlock(t.Context(), []byte("partial-data")))
	require.NoError(t, writer.Flush())
	// Close, not CloseMarkDone: the file stays under its partial name, as it
	// does when a Put stream is interrupted by a failover.
	require.NoError(t, writer.Close())

	task := &queue.WALTask{ClusterName: clusterName, WALName: walName}
	require.NoError(t, w.walHandler(t.Context(), task))

	exists, err := tier2.IsWALFileExisting(clusterName, walName)
	require.NoError(t, err)
	assert.True(t, exists, "partial WAL file must be archived to tier-2 under its partial name")
}

// TestWalHandlerArchivesHistoryFile verifies that a timeline history file is
// archived to tier-2 like any other WAL task, without erroring on the LSN and
// timeline metrics that only apply to real WAL segments.
func TestWalHandlerArchivesHistoryFile(t *testing.T) {
	const (
		clusterName = "cluster-example"
		historyName = "00000002.history"
	)

	tier1Opts := repository.Options{FS: afero.NewMemMapFs(), Password: "tier1-password"}
	require.NoError(t, repository.Initialize(tier1Opts))
	tier1, err := repository.Open(tier1Opts)
	require.NoError(t, err)
	defer tier1.Close()

	tier2Opts := repository.Options{FS: afero.NewMemMapFs(), Password: "tier2-password"}
	require.NoError(t, repository.Initialize(tier2Opts))
	tier2, err := repository.Open(tier2Opts)
	require.NoError(t, err)
	defer tier2.Close()

	w := NewWAL(&WALOptions{Tier1: tier1, Tier2: tier2})

	writer, err := tier1.NewWriter(repository.WriterOptions{
		ClusterName: clusterName,
		WALName:     historyName,
		SegmentSize: 16,
		Metrics:     w.metrics,
	})
	require.NoError(t, err)
	require.NoError(t, writer.WriteBlock(t.Context(), []byte("history-data")))
	require.NoError(t, writer.CloseMarkDone())

	task := &queue.WALTask{ClusterName: clusterName, WALName: historyName}
	require.NoError(t, w.walHandler(t.Context(), task))

	exists, err := tier2.IsWALFileExisting(clusterName, historyName)
	require.NoError(t, err)
	assert.True(t, exists, "history file must be archived to tier-2")
}

// TestLatestWrittenLSN verifies that the reported LSN reflects the bytes
// actually archived: a full segment reports the segment's end LSN, while a
// .partial segment reports a lower LSN proportional to its written size.
func TestLatestWrittenLSN(t *testing.T) {
	const (
		walName     = "000000010000000000000002"
		segmentSize = 16 * 1024 * 1024
	)

	// The start LSN of segment 2 sits at 2 * segmentSize.
	startPos := uint64(2) * segmentSize

	complete, err := latestWrittenLSN(walName, segmentSize, segmentSize)
	require.NoError(t, err)
	assert.Equal(t, startPos+segmentSize-1, complete,
		"a complete segment must report the segment's end LSN")

	const partialSize = 4 * 1024 * 1024
	partial, err := latestWrittenLSN(walName, segmentSize, partialSize)
	require.NoError(t, err)
	assert.Equal(t, startPos+partialSize-1, partial,
		"a partial segment must report the LSN of the bytes actually written")
	assert.Less(t, partial, complete,
		"a partial segment must report a lower LSN than a complete one")

	// A malformed (non-24-char) name yields an error rather than a bogus LSN.
	_, err = latestWrittenLSN(walName+".partial", segmentSize, partialSize)
	require.Error(t, err)
}

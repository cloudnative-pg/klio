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

package opentelemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// setupWalDurationProvider installs a manual-reader MeterProvider that applies
// the WAL duration bucket views and rebinds the WAL instruments against it,
// returning the reader for collection. The previous provider is restored on
// test cleanup.
func setupWalDurationProvider(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(opentelemetry.WalDurationViews()...),
	)

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
	})

	opentelemetry.InitServerWalMetrics()
	opentelemetry.InitClientWalMetrics()

	return reader
}

func collectHistogram(
	t *testing.T,
	reader *sdkmetric.ManualReader,
	name string,
) []metricdata.HistogramDataPoint[int64] {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			h, ok := m.Data.(metricdata.Histogram[int64])
			require.True(t, ok, "%s must be an int64 histogram", name)

			return h.DataPoints
		}
	}

	t.Fatalf("histogram %s not found in collected metrics", name)

	return nil
}

// TestServerWalBlockDurationStages verifies that the server per-block histogram
// keeps one data point per (stage, tier, outcome) combination and that the
// explicit bucket boundaries are applied (guarding against the +Inf collapse
// that an exponential aggregation would suffer through the Prometheus bridge).
func TestServerWalBlockDurationStages(t *testing.T) {
	reader := setupWalDurationProvider(t)
	ctx := context.Background()

	opentelemetry.ServerWal.BlockDuration.Record(ctx, 2_000_000,
		metric.WithAttributes(
			opentelemetry.Tier1.Attribute(),
			opentelemetry.StageFlush.Attribute(),
			opentelemetry.OutcomeSuccess.Attribute(),
		))
	opentelemetry.ServerWal.BlockDuration.Record(ctx, 20_000_000,
		metric.WithAttributes(
			opentelemetry.Tier1.Attribute(),
			opentelemetry.StageWrite.Attribute(),
			opentelemetry.OutcomeSuccess.Attribute(),
		))

	dps := collectHistogram(t, reader, opentelemetry.ServerWalBlockDurationMetric)
	require.Len(t, dps, 2, "expected one data point per stage")

	byStage := map[string]metricdata.HistogramDataPoint[int64]{}
	for _, dp := range dps {
		stage, ok := dp.Attributes.Value(attribute.Key("stage"))
		require.True(t, ok, "every data point must carry a stage attribute")
		tier, ok := dp.Attributes.Value(attribute.Key("tier"))
		require.True(t, ok, "every data point must carry a tier attribute")
		assert.Equal(t, string(opentelemetry.Tier1), tier.AsString())
		byStage[stage.AsString()] = dp
	}

	require.Contains(t, byStage, string(opentelemetry.StageFlush))
	require.Contains(t, byStage, string(opentelemetry.StageWrite))
	assert.Equal(t, uint64(1), byStage[string(opentelemetry.StageFlush)].Count)

	// The block-level ladder must be in force, not a single +Inf bucket.
	assert.Greater(t, len(byStage[string(opentelemetry.StageFlush)].Bounds), 1,
		"explicit bucket boundaries must be applied to the block histogram")
}

// TestServerWalFileDurations verifies the per-file get/upload histograms
// record with the expected tier and outcome attributes.
func TestServerWalFileDurations(t *testing.T) {
	reader := setupWalDurationProvider(t)
	ctx := context.Background()

	opentelemetry.ServerWal.GetDuration.Record(ctx, 250_000_000,
		metric.WithAttributes(opentelemetry.Tier1.Attribute(), opentelemetry.OutcomeSuccess.Attribute()))
	opentelemetry.ServerWal.UploadDuration.Record(ctx, 2_000_000_000,
		metric.WithAttributes(opentelemetry.Tier2.Attribute(), opentelemetry.OutcomeSuccess.Attribute()))

	get := collectHistogram(t, reader, opentelemetry.ServerWalGetDurationMetric)
	require.Len(t, get, 1)
	tier, ok := get[0].Attributes.Value(attribute.Key("tier"))
	require.True(t, ok)
	assert.Equal(t, string(opentelemetry.Tier1), tier.AsString())

	upload := collectHistogram(t, reader, opentelemetry.ServerWalUploadDurationMetric)
	require.Len(t, upload, 1)
	tier, ok = upload[0].Attributes.Value(attribute.Key("tier"))
	require.True(t, ok)
	assert.Equal(t, string(opentelemetry.Tier2), tier.AsString())
}

// TestClientWalBlockDurationSend verifies the client per-block histogram records
// the send stage with an outcome attribute.
func TestClientWalBlockDurationSend(t *testing.T) {
	reader := setupWalDurationProvider(t)
	ctx := context.Background()

	opentelemetry.ClientWal.BlockDuration.Record(ctx, 10_000_000,
		metric.WithAttributes(opentelemetry.StageSend.Attribute(), opentelemetry.OutcomeSuccess.Attribute()))

	dps := collectHistogram(t, reader, opentelemetry.ClientWalBlockDurationMetric)
	require.Len(t, dps, 1)
	stage, ok := dps[0].Attributes.Value(attribute.Key("stage"))
	require.True(t, ok)
	assert.Equal(t, string(opentelemetry.StageSend), stage.AsString())
}

// TestClientWalTimeline verifies the client timeline gauge reports the most
// recently recorded timeline value.
func TestClientWalTimeline(t *testing.T) {
	reader := setupWalDurationProvider(t)
	ctx := context.Background()

	opentelemetry.ClientWal.Timeline.Record(ctx, 3)
	opentelemetry.ClientWal.Timeline.Record(ctx, 4)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	var dps []metricdata.DataPoint[int64]
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ClientWalTimelineMetric {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "%s must be an int64 gauge", m.Name)
			dps = g.DataPoints
		}
	}

	require.Len(t, dps, 1, "streaming timeline gauge keeps a single series")
	assert.Equal(t, int64(4), dps[0].Value, "gauge must report the latest timeline")
}

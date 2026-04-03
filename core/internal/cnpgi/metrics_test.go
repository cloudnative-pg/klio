package cnpgi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// setupTestMeter installs a test MeterProvider with a ManualReader,
// re-creates all instruments against it, and returns the reader.
func setupTestMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
	})

	opentelemetry.InitBackupMetrics()

	return reader
}

// collectOTelMetrics triggers a manual collection and returns ResourceMetrics.
func collectOTelMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	return rm
}

func findFloat64GaugeValue(rm metricdata.ResourceMetrics, name string) (float64, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if g, ok := m.Data.(metricdata.Gauge[float64]); ok && len(g.DataPoints) > 0 {
					return g.DataPoints[0].Value, true
				}
			}
		}
	}

	return 0, false
}

func findInt64GaugeValue(rm metricdata.ResourceMetrics, name string) (int64, bool) { //nolint:unparam
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if g, ok := m.Data.(metricdata.Gauge[int64]); ok && len(g.DataPoints) > 0 {
					return g.DataPoints[0].Value, true
				}
			}
		}
	}

	return 0, false
}

func findInt64CounterValue(rm metricdata.ResourceMetrics, name string) (int64, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if s, ok := m.Data.(metricdata.Sum[int64]); ok && len(s.DataPoints) > 0 {
					return s.DataPoints[0].Value, true
				}
			}
		}
	}

	return 0, false
}

func findInt64SumDataPoints(
	rm metricdata.ResourceMetrics, name string,
) []metricdata.DataPoint[int64] {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if s, ok := m.Data.(metricdata.Sum[int64]); ok {
					return s.DataPoints
				}
			}
		}
	}

	return nil
}

func TestRecordBackupStart(t *testing.T) {
	reader := setupTestMeter(t)

	before := time.Now().Unix()
	recordBackupStart(context.Background())

	rm := collectOTelMetrics(t, reader)

	startTime, ok := findFloat64GaugeValue(rm, opentelemetry.BackupLatestStartTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, startTime, float64(before))

	running, ok := findInt64GaugeValue(rm, opentelemetry.BackupRunningMetric)
	require.True(t, ok)
	assert.Equal(t, int64(1), running)
}

func TestRecordBackupSuccess(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupStart(context.Background())

	duration := 42 * time.Second
	before := time.Now().Unix()
	recordBackupSuccess(context.Background(), duration)

	rm := collectOTelMetrics(t, reader)

	completionTime, ok := findFloat64GaugeValue(rm, opentelemetry.BackupLatestCompletionTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, completionTime, float64(before))

	latestDuration, ok := findFloat64GaugeValue(rm, opentelemetry.BackupLatestDurationMetric)
	require.True(t, ok)
	assert.InDelta(t, 42.0, latestDuration, 0.01)

	running, ok := findInt64GaugeValue(rm, opentelemetry.BackupRunningMetric)
	require.True(t, ok)
	assert.Equal(t, int64(0), running)

	successes, ok := findInt64CounterValue(rm, opentelemetry.BackupSuccessesMetric)
	require.True(t, ok)
	assert.Equal(t, int64(1), successes)
}

func TestRecordBackupFailure(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupStart(context.Background())

	before := time.Now().Unix()
	recordBackupFailure(context.Background())

	rm := collectOTelMetrics(t, reader)

	failureTime, ok := findFloat64GaugeValue(rm, opentelemetry.BackupLatestFailureTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, failureTime, float64(before))

	running, ok := findInt64GaugeValue(rm, opentelemetry.BackupRunningMetric)
	require.True(t, ok)
	assert.Equal(t, int64(0), running)

	failures, ok := findInt64CounterValue(rm, opentelemetry.BackupFailuresMetric)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)
}

func TestBackupMetricsMultipleRuns(t *testing.T) {
	reader := setupTestMeter(t)

	// First backup: success.
	recordBackupStart(context.Background())
	recordBackupSuccess(context.Background(), 10*time.Second)

	// Second backup: failure.
	recordBackupStart(context.Background())
	recordBackupFailure(context.Background())

	// Third backup: success.
	recordBackupStart(context.Background())
	recordBackupSuccess(context.Background(), 30*time.Second)

	rm := collectOTelMetrics(t, reader)

	successes, ok := findInt64CounterValue(rm, opentelemetry.BackupSuccessesMetric)
	require.True(t, ok)
	assert.Equal(t, int64(2), successes)

	failures, ok := findInt64CounterValue(rm, opentelemetry.BackupFailuresMetric)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	// Latest duration should reflect the last successful backup.
	latestDuration, ok := findFloat64GaugeValue(rm, opentelemetry.BackupLatestDurationMetric)
	require.True(t, ok)
	assert.InDelta(t, 30.0, latestDuration, 0.01)

	running, ok := findInt64GaugeValue(rm, opentelemetry.BackupRunningMetric)
	require.True(t, ok)
	assert.Equal(t, int64(0), running)
}

func TestRecordVerificationSuccess(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationSuccess(context.Background())
	recordVerificationSuccess(context.Background())

	rm := collectOTelMetrics(t, reader)

	totalDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationsMetric)
	require.Len(t, totalDPs, 1)
	assert.Equal(t, int64(2), totalDPs[0].Value)

	// OTel SDK does not report counters that were never incremented.
	failDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationFailuresMetric)
	assert.Empty(t, failDPs)
}

func TestRecordVerificationFailure(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationFailure(context.Background())

	rm := collectOTelMetrics(t, reader)

	totalDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationsMetric)
	require.Len(t, totalDPs, 1)
	assert.Equal(t, int64(1), totalDPs[0].Value)

	failDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationFailuresMetric)
	require.Len(t, failDPs, 1)
	assert.Equal(t, int64(1), failDPs[0].Value)
}

func TestRecordMixed(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationSuccess(context.Background())
	recordVerificationFailure(context.Background())
	recordVerificationSuccess(context.Background())

	rm := collectOTelMetrics(t, reader)

	totalDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationsMetric)
	require.Len(t, totalDPs, 1)
	assert.Equal(t, int64(3), totalDPs[0].Value)

	failDPs := findInt64SumDataPoints(rm, opentelemetry.BackupVerificationFailuresMetric)
	require.Len(t, failDPs, 1)
	assert.Equal(t, int64(1), failDPs[0].Value)
}

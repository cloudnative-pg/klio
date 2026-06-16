package cnpgi

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/backupfailure"
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

	opentelemetry.InitPluginBackupMetrics()

	return reader
}

// collectOTelMetrics triggers a manual collection and returns ResourceMetrics.
func collectOTelMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	return rm
}

//nolint:ireturn
func findGaugeValue[N int64 | float64](rm metricdata.ResourceMetrics, name string) (N, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if g, ok := m.Data.(metricdata.Gauge[N]); ok && len(g.DataPoints) > 0 {
					return g.DataPoints[0].Value, true
				}
			}
		}
	}

	return 0, false
}

// findInProgressValue returns the current value of the
// `klio.plugin.backup.in_progress` UpDownCounter, or (0, false) if no data
// point has been recorded yet.
func findInProgressValue(rm metricdata.ResourceMetrics) (int64, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.PluginBackupInProgressMetric {
				continue
			}
			if s, ok := m.Data.(metricdata.Sum[int64]); ok && len(s.DataPoints) > 0 {
				return s.DataPoints[0].Value, true
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

// findInt64SumValueByOutcome returns the value of the data point on the named
// Int64 Sum instrument that carries the given `outcome` attribute, or
// (0, false) if no such data point exists.
func findInt64SumValueByOutcome(
	rm metricdata.ResourceMetrics, name string, outcome opentelemetry.Outcome,
) (int64, bool) {
	var total int64
	var found bool
	for _, dp := range findInt64SumDataPoints(rm, name) {
		v, ok := dp.Attributes.Value(attribute.Key("outcome"))
		if !ok {
			continue
		}
		if v.AsString() == string(outcome) {
			total += dp.Value
			found = true
		}
	}

	return total, found
}

// findInt64SumValueByFailureCategory returns the value of the data point on
// the named Int64 Sum instrument that carries the given `failure_category`
// attribute, or (0, false) if no such data point exists.
func findInt64SumValueByFailureCategory(
	rm metricdata.ResourceMetrics, name string, category backupfailure.Category,
) (int64, bool) {
	for _, dp := range findInt64SumDataPoints(rm, name) {
		v, ok := dp.Attributes.Value(attribute.Key("failure_category"))
		if !ok {
			continue
		}
		if v.AsString() == category.Name {
			return dp.Value, true
		}
	}

	return 0, false
}

func TestRecordBackupStart(t *testing.T) {
	reader := setupTestMeter(t)

	before := time.Now().Unix()
	recordBackupStart(context.Background())

	rm := collectOTelMetrics(t, reader)

	startTime, ok := findGaugeValue[int64](rm, opentelemetry.PluginBackupLatestStartTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, startTime, before)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(1), inProgress)
}

func TestRecordBackupSuccess(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupStart(context.Background())
	duration := 42 * time.Second
	before := time.Now().Unix()
	recordBackupSuccess(context.Background(), duration)
	recordBackupFinished(context.Background())

	rm := collectOTelMetrics(t, reader)

	completionTime, ok := findGaugeValue[int64](rm, opentelemetry.PluginBackupLatestCompletionTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, completionTime, before)

	latestDuration, ok := findGaugeValue[float64](rm, opentelemetry.PluginBackupLatestDurationMetric)
	require.True(t, ok)
	assert.InDelta(t, 42.0, latestDuration, 0.01)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(0), inProgress)

	successes, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(1), successes)

	_, failPresent := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeFailure)
	assert.False(t, failPresent, "no failure data point should be emitted on a clean success")
}

func TestRecordBackupFailure(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupStart(t.Context())
	before := time.Now().Unix()

	//nolint:gosec // hardcoded test input
	exitErr := exec.CommandContext(t.Context(), "sh",
		"-c", fmt.Sprintf("exit %d", backupfailure.RepositoryError.ExitCode)).Run()

	recordBackupFailure(t.Context(), exitErr)
	recordBackupFinished(t.Context())

	rm := collectOTelMetrics(t, reader)

	failureTime, ok := findGaugeValue[int64](rm, opentelemetry.PluginBackupLatestFailureTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, failureTime, before)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(0), inProgress)

	failures, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	byCat, ok := findInt64SumValueByFailureCategory(
		rm, opentelemetry.PluginBackupRunsMetric, backupfailure.RepositoryError)
	require.True(t, ok)
	assert.Equal(t, int64(1), byCat)
}

func TestRecordBackupFailureCategoriesAreSeparateSeries(t *testing.T) {
	reader := setupTestMeter(t)
	expiredCtx, cancelTimeout := context.WithTimeout(t.Context(), 0)
	defer cancelTimeout()

	recordBackupFailure(expiredCtx, nil)
	canceledCtx, cancelCanceled := context.WithCancel(t.Context())
	cancelCanceled()
	recordBackupFailure(canceledCtx, nil)
	recordBackupFailure(canceledCtx, nil)

	//nolint:gosec // hardcoded test input
	exitErr := exec.CommandContext(t.Context(), "sh",
		"-c", fmt.Sprintf("exit %d", backupfailure.Verification.ExitCode)).Run()
	recordBackupFailure(t.Context(), exitErr)

	rm := collectOTelMetrics(t, reader)

	for category, want := range map[backupfailure.Category]int64{
		backupfailure.Timeout:      1,
		backupfailure.Canceled:     2,
		backupfailure.Verification: 1,
	} {
		got, ok := findInt64SumValueByFailureCategory(
			rm, opentelemetry.PluginBackupRunsMetric, category)
		require.True(t, ok, "expected data point for category %q", category.Name)
		assert.Equal(t, want, got, "category %q", category.Name)
	}

	totalFailures, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(4), totalFailures)
}

func TestRecordBackupFailureNilErrorDefaultsToUnknown(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupFailure(context.Background(), nil)

	rm := collectOTelMetrics(t, reader)

	unknown, ok := findInt64SumValueByFailureCategory(
		rm, opentelemetry.PluginBackupRunsMetric, backupfailure.Unknown)
	require.True(t, ok)
	assert.Equal(t, int64(1), unknown)
}

func TestBackupMetricsMultipleRuns(t *testing.T) {
	reader := setupTestMeter(t)

	// First backup: success.
	recordBackupStart(context.Background())
	recordBackupSuccess(context.Background(), 10*time.Second)
	recordBackupFinished(context.Background())

	// Second backup: failure.
	recordBackupStart(context.Background())
	recordBackupFailure(context.Background(), assert.AnError)
	recordBackupFinished(context.Background())

	// Third backup: success.
	recordBackupStart(context.Background())
	recordBackupSuccess(context.Background(), 30*time.Second)
	recordBackupFinished(context.Background())

	rm := collectOTelMetrics(t, reader)

	successes, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(2), successes)

	failures, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupRunsMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	// Latest duration should reflect the last successful backup.
	latestDuration, ok := findGaugeValue[float64](rm, opentelemetry.PluginBackupLatestDurationMetric)
	require.True(t, ok)
	assert.InDelta(t, 30.0, latestDuration, 0.01)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(0), inProgress)
}

func TestRecordVerificationSuccess(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationSuccess(context.Background())
	recordVerificationSuccess(context.Background())

	rm := collectOTelMetrics(t, reader)

	successes, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(2), successes)

	_, failPresent := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeFailure)
	assert.False(t, failPresent, "no failure data point should be emitted when no failure happens")
}

func TestRecordVerificationFailure(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationFailure(context.Background())

	rm := collectOTelMetrics(t, reader)

	failures, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	_, successPresent := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeSuccess)
	assert.False(t, successPresent, "no success data point should be emitted when no success happens")
}

func TestRecordMixed(t *testing.T) {
	reader := setupTestMeter(t)

	recordVerificationSuccess(context.Background())
	recordVerificationFailure(context.Background())
	recordVerificationSuccess(context.Background())

	rm := collectOTelMetrics(t, reader)

	successes, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(2), successes)

	failures, ok := findInt64SumValueByOutcome(
		rm, opentelemetry.PluginBackupVerificationsMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)
}

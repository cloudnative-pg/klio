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

// findBackupInt64SumValueByOutcome returns the value of the data point on the
// backup runs Int64 Sum instrument that carries the given `outcome`
// attribute, or (0, false) if no such data point exists.
func findBackupInt64SumValueByOutcome(
	rm metricdata.ResourceMetrics, outcome opentelemetry.Outcome,
) (int64, bool) {
	var total int64
	var found bool
	for _, dp := range findInt64SumDataPoints(rm, opentelemetry.PluginBackupRunsMetric) {
		v, ok := dp.Attributes.Value("outcome")
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

// findBackupInt64SumValueByFailureCategory returns the value of the data point on
// the backup runs Int64 Sum instrument that carries the given
// `failure_category` attribute, or (0, false) if no such data point exists.
func findBackupInt64SumValueByFailureCategory(
	rm metricdata.ResourceMetrics, category backupfailure.Category,
) (int64, bool) {
	for _, dp := range findInt64SumDataPoints(rm, opentelemetry.PluginBackupRunsMetric) {
		v, ok := dp.Attributes.Value("failure_category")
		if !ok {
			continue
		}
		if v.AsString() == category.Name {
			return dp.Value, true
		}
	}

	return 0, false
}

func findFloat64HistogramDataPoints(
	rm metricdata.ResourceMetrics, name string,
) []metricdata.HistogramDataPoint[float64] {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if h, ok := m.Data.(metricdata.Histogram[float64]); ok {
					return h.DataPoints
				}
			}
		}
	}

	return nil
}

// findHistogramByOutcome returns (count, sum) for the data point on the named
// Float64 Histogram instrument that carries the given `outcome` attribute, or
// (0, 0, false) if no such data point exists.
//
//nolint:unparam
func findHistogramByOutcome(
	rm metricdata.ResourceMetrics, name string, outcome opentelemetry.Outcome,
) (uint64, float64, bool) {
	for _, dp := range findFloat64HistogramDataPoints(rm, name) {
		v, ok := dp.Attributes.Value("outcome")
		if !ok {
			continue
		}
		if v.AsString() == string(outcome) {
			return dp.Count, dp.Sum, true
		}
	}

	return 0, 0, false
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

	successes, ok := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(1), successes)

	_, failPresent := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeFailure)
	assert.False(t, failPresent, "no failure data point should be emitted on a clean success")

	histCount, histSum, ok := findHistogramByOutcome(
		rm, opentelemetry.PluginBackupDurationMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, uint64(1), histCount)
	assert.InDelta(t, 42.0, histSum, 0.01)
}

func TestRecordBackupFailure(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupStart(t.Context())
	before := time.Now().Unix()

	//nolint:gosec // hardcoded test input
	exitErr := exec.CommandContext(t.Context(), "sh",
		"-c", fmt.Sprintf("exit %d", backupfailure.RepositoryError.ExitCode)).Run()

	recordBackupFailure(t.Context(), 7*time.Second, exitErr)
	recordBackupFinished(t.Context())

	rm := collectOTelMetrics(t, reader)

	failureTime, ok := findGaugeValue[int64](rm, opentelemetry.PluginBackupLatestFailureTimeMetric)
	require.True(t, ok)
	assert.GreaterOrEqual(t, failureTime, before)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(0), inProgress)

	failures, ok := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	histCount, histSum, ok := findHistogramByOutcome(
		rm, opentelemetry.PluginBackupDurationMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, uint64(1), histCount)
	assert.InDelta(t, 7.0, histSum, 0.01)

	byCat, ok := findBackupInt64SumValueByFailureCategory(
		rm, backupfailure.RepositoryError)
	require.True(t, ok)
	assert.Equal(t, int64(1), byCat)
}

func TestRecordBackupFailureCategoriesAreSeparateSeries(t *testing.T) {
	reader := setupTestMeter(t)
	expiredCtx, cancelTimeout := context.WithTimeout(t.Context(), 0)
	defer cancelTimeout()

	recordBackupFailure(expiredCtx, time.Second, nil)
	canceledCtx, cancelCanceled := context.WithCancel(t.Context())
	cancelCanceled()
	recordBackupFailure(canceledCtx, time.Second, nil)
	recordBackupFailure(canceledCtx, time.Second, nil)

	//nolint:gosec // hardcoded test input
	exitErr := exec.CommandContext(t.Context(), "sh",
		"-c", fmt.Sprintf("exit %d", backupfailure.Verification.ExitCode)).Run()
	recordBackupFailure(t.Context(), time.Second, exitErr)

	rm := collectOTelMetrics(t, reader)

	for category, want := range map[backupfailure.Category]int64{
		backupfailure.Timeout:      1,
		backupfailure.Canceled:     2,
		backupfailure.Verification: 1,
	} {
		got, ok := findBackupInt64SumValueByFailureCategory(
			rm, category)
		require.True(t, ok, "expected data point for category %q", category.Name)
		assert.Equal(t, want, got, "category %q", category.Name)
	}

	totalFailures, ok := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(4), totalFailures)
}

func TestRecordBackupFailureNilErrorDefaultsToUnknown(t *testing.T) {
	reader := setupTestMeter(t)

	recordBackupFailure(context.Background(), time.Second, nil)

	rm := collectOTelMetrics(t, reader)

	unknown, ok := findBackupInt64SumValueByFailureCategory(
		rm, backupfailure.Unknown)
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
	recordBackupFailure(context.Background(), 3*time.Second, assert.AnError)
	recordBackupFinished(context.Background())

	// Third backup: success.
	recordBackupStart(context.Background())
	recordBackupSuccess(context.Background(), 30*time.Second)
	recordBackupFinished(context.Background())

	rm := collectOTelMetrics(t, reader)

	successes, ok := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, int64(2), successes)

	failures, ok := findBackupInt64SumValueByOutcome(
		rm, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, int64(1), failures)

	// Latest duration should reflect the last successful backup.
	latestDuration, ok := findGaugeValue[float64](rm, opentelemetry.PluginBackupLatestDurationMetric)
	require.True(t, ok)
	assert.InDelta(t, 30.0, latestDuration, 0.01)

	inProgress, ok := findInProgressValue(rm)
	require.True(t, ok)
	assert.Equal(t, int64(0), inProgress)

	// Histogram aggregates both successful runs (10s + 30s).
	successCount, successSum, ok := findHistogramByOutcome(
		rm, opentelemetry.PluginBackupDurationMetric, opentelemetry.OutcomeSuccess)
	require.True(t, ok)
	assert.Equal(t, uint64(2), successCount)
	assert.InDelta(t, 40.0, successSum, 0.01)

	// Failure-path duration is also observed.
	failureCount, failureSum, ok := findHistogramByOutcome(
		rm, opentelemetry.PluginBackupDurationMetric, opentelemetry.OutcomeFailure)
	require.True(t, ok)
	assert.Equal(t, uint64(1), failureCount)
	assert.InDelta(t, 3.0, failureSum, 0.01)
}

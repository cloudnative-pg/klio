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

package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// histogramExposition is a Prometheus exposition payload containing an
// explicit-bucket histogram (`success`), a histogram collapsed to the `+Inf`
// bucket only (`failure`), plus a counter and a gauge.
const histogramExposition = `# HELP klio_plugin_backup_duration_seconds Backup durations.
# TYPE klio_plugin_backup_duration_seconds histogram
klio_plugin_backup_duration_seconds_bucket{outcome="success",le="1"} 0
klio_plugin_backup_duration_seconds_bucket{outcome="success",le="5"} 1
klio_plugin_backup_duration_seconds_bucket{outcome="success",le="10"} 2
klio_plugin_backup_duration_seconds_bucket{outcome="success",le="+Inf"} 3
klio_plugin_backup_duration_seconds_sum{outcome="success"} 42
klio_plugin_backup_duration_seconds_count{outcome="success"} 3
# HELP klio_plugin_backup_collapsed_seconds Collapsed exponential histogram.
# TYPE klio_plugin_backup_collapsed_seconds histogram
klio_plugin_backup_collapsed_seconds_bucket{outcome="failure",le="+Inf"} 2
klio_plugin_backup_collapsed_seconds_sum{outcome="failure"} 9
klio_plugin_backup_collapsed_seconds_count{outcome="failure"} 2
# HELP klio_plugin_backup_runs_total Backup runs.
# TYPE klio_plugin_backup_runs_total counter
klio_plugin_backup_runs_total{outcome="success"} 5
# HELP klio_plugin_backup_in_progress Backups in progress.
# TYPE klio_plugin_backup_in_progress gauge
klio_plugin_backup_in_progress 0
`

func parseFixture(t *testing.T) PrometheusMetrics {
	t.Helper()
	m, err := ParsePrometheusMetrics(strings.NewReader(histogramExposition))
	require.NoError(t, err)

	return m
}

func TestParsePrometheusMetricsHistogram(t *testing.T) {
	m := parseFixture(t)

	// The histogram family is surfaced as a single metric whose Value mirrors
	// the sample count for convenience with the value-based helpers.
	require.True(t, m.HasMetric("klio_plugin_backup_duration_seconds"))
	value, ok := m.GetValue("klio_plugin_backup_duration_seconds")
	require.True(t, ok)
	assert.InDelta(t, 3.0, value, 0.001)

	// Counters and gauges keep their scalar value and carry no histogram data.
	runs, ok := m.GetValueWithLabels(
		"klio_plugin_backup_runs_total", map[string]string{"outcome": "success"})
	require.True(t, ok)
	assert.InDelta(t, 5.0, runs, 0.001)

	_, ok = m.GetHistogram("klio_plugin_backup_runs_total", nil)
	assert.False(t, ok, "a counter must not be reported as a histogram")
}

func TestGetHistogram(t *testing.T) {
	m := parseFixture(t)

	hist, ok := m.GetHistogram(
		"klio_plugin_backup_duration_seconds", map[string]string{"outcome": "success"})
	require.True(t, ok)
	assert.Equal(t, uint64(3), hist.SampleCount)
	assert.InDelta(t, 42.0, hist.SampleSum, 0.001)
	// Three finite boundaries plus the +Inf bucket.
	assert.Equal(t, 4, hist.BucketCount)
}

func TestGetHistogramCollapsed(t *testing.T) {
	m := parseFixture(t)

	// A histogram exposed with only the +Inf bucket yields a BucketCount of one,
	// which is how a base-2 exponential histogram degrades through the classic
	// Prometheus exposition format.
	hist, ok := m.GetHistogram(
		"klio_plugin_backup_collapsed_seconds", map[string]string{"outcome": "failure"})
	require.True(t, ok)
	assert.Equal(t, uint64(2), hist.SampleCount)
	assert.Equal(t, 1, hist.BucketCount)
}

func TestGetHistogramLabelMismatch(t *testing.T) {
	m := parseFixture(t)

	_, ok := m.GetHistogram(
		"klio_plugin_backup_duration_seconds", map[string]string{"outcome": "failure"})
	assert.False(t, ok, "no histogram carries the requested label set")
}

func TestGetHistogramNotFound(t *testing.T) {
	m := parseFixture(t)

	_, ok := m.GetHistogram("klio_plugin_backup_missing_seconds", nil)
	assert.False(t, ok)
}

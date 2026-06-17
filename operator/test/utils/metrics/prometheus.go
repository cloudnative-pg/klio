// Package metrics provides utilities for parsing and validating telemetry metrics in e2e tests.
package metrics

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// PrometheusMetric represents a parsed Prometheus metric.
type PrometheusMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
	// Histogram holds the aggregated histogram data when the metric is a
	// histogram; it is nil for every other metric type.
	Histogram *HistogramData
}

// HistogramData holds the aggregated values of a Prometheus histogram metric.
type HistogramData struct {
	SampleCount uint64
	SampleSum   float64
	// BucketCount is the number of emitted `le` buckets (including `+Inf`). A
	// value greater than one proves the distribution survived the exposition
	// format, as opposed to an exponential histogram collapsed to `+Inf` only.
	BucketCount int
}

// PrometheusMetrics is a collection of parsed Prometheus metrics.
type PrometheusMetrics []PrometheusMetric

// ParsePrometheusMetrics parses Prometheus exposition format text into structured metrics.
func ParsePrometheusMetrics(r io.Reader) (PrometheusMetrics, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(r)
	if err != nil {
		return nil, fmt.Errorf("error parsing metrics: %w", err)
	}

	var metrics PrometheusMetrics
	for name, family := range families {
		for _, m := range family.GetMetric() {
			pm := PrometheusMetric{
				Name:   name,
				Labels: make(map[string]string, len(m.GetLabel())),
			}
			for _, lp := range m.GetLabel() {
				pm.Labels[lp.GetName()] = lp.GetValue()
			}
			// Extract value based on metric type
			switch {
			case m.GetGauge() != nil:
				pm.Value = m.GetGauge().GetValue()
			case m.GetCounter() != nil:
				pm.Value = m.GetCounter().GetValue()
			case m.GetUntyped() != nil:
				pm.Value = m.GetUntyped().GetValue()
			case m.GetHistogram() != nil:
				h := m.GetHistogram()
				pm.Value = float64(h.GetSampleCount())
				pm.Histogram = &HistogramData{
					SampleCount: h.GetSampleCount(),
					SampleSum:   h.GetSampleSum(),
					BucketCount: len(h.GetBucket()),
				}
			}
			metrics = append(metrics, pm)
		}
	}

	return metrics, nil
}

// FindByName returns all metrics with the given name.
func (m PrometheusMetrics) FindByName(name string) PrometheusMetrics {
	var result PrometheusMetrics
	for _, metric := range m {
		if metric.Name == name {
			result = append(result, metric)
		}
	}

	return result
}

// FindByNamePrefix returns all metrics whose name starts with the given prefix.
func (m PrometheusMetrics) FindByNamePrefix(prefix string) PrometheusMetrics {
	var result PrometheusMetrics
	for _, metric := range m {
		if strings.HasPrefix(metric.Name, prefix) {
			result = append(result, metric)
		}
	}

	return result
}

// GetValue returns the value of the first metric with the given name.
// Returns 0 and false if not found.
func (m PrometheusMetrics) GetValue(name string) (float64, bool) {
	for _, metric := range m {
		if metric.Name == name {
			return metric.Value, true
		}
	}

	return 0, false
}

// GetValueWithLabels returns the value of the metric matching name and all specified labels.
// Returns 0 and false if not found.
func (m PrometheusMetrics) GetValueWithLabels(name string, labels map[string]string) (float64, bool) {
	for _, metric := range m {
		if metric.Name != name {
			continue
		}

		match := true
		for k, v := range labels {
			if metric.Labels[k] != v {
				match = false
				break
			}
		}

		if match {
			return metric.Value, true
		}
	}

	return 0, false
}

// hasLabels reports whether the metric carries every label in the given set.
func (p PrometheusMetric) hasLabels(labels map[string]string) bool {
	for k, v := range labels {
		if p.Labels[k] != v {
			return false
		}
	}

	return true
}

// GetHistogram returns the histogram data for the metric matching name and all
// specified labels. Returns nil and false if not found or not a histogram.
func (m PrometheusMetrics) GetHistogram(
	name string, labels map[string]string,
) (*HistogramData, bool) {
	idx := slices.IndexFunc(m, func(metric PrometheusMetric) bool {
		return metric.Name == name && metric.Histogram != nil && metric.hasLabels(labels)
	})
	if idx == -1 {
		return nil, false
	}

	return m[idx].Histogram, true
}

// HasMetric returns true if a metric with the given name exists.
func (m PrometheusMetrics) HasMetric(name string) bool {
	_, found := m.GetValue(name)

	return found
}

// Names returns all unique metric names.
func (m PrometheusMetrics) Names() []string {
	set := make(map[string]struct{})

	for _, metric := range m {
		set[metric.Name] = struct{}{}
	}

	return slices.Sorted(maps.Keys(set))
}

package repository

import (
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds OpenTelemetry metrics for the repository operations.
type Metrics struct {
	WalWrittenBytes       metric.Int64Counter
	WalWritten            metric.Int64Counter
	LatestWrittenTime     metric.Int64Gauge
	LatestWrittenLSN      metric.Int64Gauge
	LatestWrittenTimeline metric.Int64Gauge
	// Attributes are merged into every recording made through this Metrics.
	// Callers use this to attach tier="1" or tier="2" to the unified WAL
	// instruments depending on which side of the WAL pipeline they sit on.
	Attributes []attribute.KeyValue
}

// AttributeSet returns the canonical attribute.Set obtained by merging
// the per-recording extras onto the Metrics' base Attributes.
func (m *Metrics) AttributeSet(extra ...attribute.KeyValue) attribute.Set {
	return attribute.NewSet(slices.Concat(m.Attributes, extra)...)
}

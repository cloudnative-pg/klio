package repository

import "go.opentelemetry.io/otel/metric"

// Metrics holds OpenTelemetry metrics for the repository operations.
type Metrics struct {
	WalWrittenBytes metric.Int64Counter
	WalWritten      metric.Int64Counter
}

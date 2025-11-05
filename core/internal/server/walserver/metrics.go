package walserver

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// NewMetrics creates a new Metrics instance with initialized counters.
func NewMetrics() *repository.Metrics {
	meter := otel.Meter(opentelemetry.Meter)

	walWrittenBytes, _ := meter.Int64Counter(
		"klio.wal.written_size",
		metric.WithDescription("Number of bytes written to disk for the WAL files"),
		metric.WithUnit("By"),
	)
	walWritten, _ := meter.Int64Counter(
		"klio.wal.written",
		metric.WithDescription("Number of WAL files written"),
		metric.WithUnit("{wals}"),
	)

	return &repository.Metrics{
		WalWrittenBytes: walWrittenBytes,
		WalWritten:      walWritten,
	}
}

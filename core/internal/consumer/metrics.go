package consumer

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

//nolint:gochecknoglobals
var (
	verificationSuccessCounter metric.Int64Counter
	verificationFailureCounter metric.Int64Counter
)

//nolint:gochecknoinits
func init() {
	meter := otel.Meter(opentelemetry.Meter)

	verificationSuccessCounter, _ = meter.Int64Counter(
		"klio.consumer.backup_verification_success",
		metric.WithDescription("Number of successful backup verifications"),
		metric.WithUnit("{verifications}"),
	)
	verificationFailureCounter, _ = meter.Int64Counter(
		"klio.consumer.backup_verification_failure",
		metric.WithDescription("Number of failed backup verifications (corruption detected)"),
		metric.WithUnit("{verifications}"),
	)
}

func recordVerificationSuccess(ctx context.Context, tier string) {
	verificationSuccessCounter.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.TierAttribute(tier)))
}

func recordVerificationFailure(ctx context.Context, tier string) {
	verificationFailureCounter.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.TierAttribute(tier)))
}

// NewMetrics creates a new Metrics instance with initialized counters.
func NewMetrics() *repository.Metrics {
	meter := otel.Meter(opentelemetry.Meter)

	walWrittenBytes, _ := meter.Int64Counter(
		"klio.consumer.written_size",
		metric.WithDescription("Number of bytes written to Tier 2 for the WAL files"),
		metric.WithUnit("By"),
	)
	walWritten, _ := meter.Int64Counter(
		"klio.consumer.written",
		metric.WithDescription("Number of WAL files written"),
		metric.WithUnit("{wals}"),
	)

	return &repository.Metrics{
		WalWrittenBytes: walWrittenBytes,
		WalWritten:      walWritten,
	}
}

package consumer

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

//nolint:gochecknoinits
func init() {
	opentelemetry.InitConsumerMetrics()
}

func recordVerificationSuccess(ctx context.Context, tier string) {
	opentelemetry.Consumer.VerificationSuccess.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.TierAttribute(tier)))
}

func recordVerificationFailure(ctx context.Context, tier string) {
	opentelemetry.Consumer.VerificationFailure.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.TierAttribute(tier)))
}

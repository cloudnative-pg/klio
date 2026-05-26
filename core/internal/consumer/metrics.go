package consumer

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

func recordVerificationSuccess(ctx context.Context, tier opentelemetry.Tier) {
	opentelemetry.ServerBackup.Verifications.Add(ctx, 1,
		metric.WithAttributes(
			tier.Attribute(),
			opentelemetry.OutcomeSuccess.Attribute(),
		))
}

func recordVerificationFailure(ctx context.Context, tier opentelemetry.Tier) {
	opentelemetry.ServerBackup.Verifications.Add(ctx, 1,
		metric.WithAttributes(
			tier.Attribute(),
			opentelemetry.OutcomeFailure.Attribute(),
		))
}

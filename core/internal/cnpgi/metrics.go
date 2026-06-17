package cnpgi

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// recordBackupStart records that a backup has started. Callers must pair this
// with a deferred recordBackupFinished so the in-progress counter decrements
// on every exit path, including panics.
func recordBackupStart(ctx context.Context) {
	opentelemetry.PluginBackup.LatestStartTime.Record(ctx, time.Now().Unix())
	opentelemetry.PluginBackup.InProgress.Add(ctx, 1)
}

// recordBackupFinished decrements the in-progress counter. Always invoke via
// defer immediately after recordBackupStart so concurrent backup accounting
// stays correct even when a backup panics or returns early.
func recordBackupFinished(ctx context.Context) {
	opentelemetry.PluginBackup.InProgress.Add(ctx, -1)
}

// recordBackupSuccess records a successful backup completion.
func recordBackupSuccess(ctx context.Context, duration time.Duration) {
	opentelemetry.PluginBackup.LatestCompletionTime.Record(ctx, time.Now().Unix())
	opentelemetry.PluginBackup.LatestDuration.Record(ctx, duration.Seconds())
	opentelemetry.PluginBackup.Duration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(opentelemetry.OutcomeSuccess.Attribute()))
	opentelemetry.PluginBackup.Runs.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.OutcomeSuccess.Attribute()))
}

// recordBackupFailure records a failed backup.
func recordBackupFailure(ctx context.Context, duration time.Duration, err error) {
	category := classifyRunBackupError(ctx, err)
	opentelemetry.PluginBackup.LatestFailureTime.Record(ctx, time.Now().Unix())
	opentelemetry.PluginBackup.Duration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(opentelemetry.OutcomeFailure.Attribute()))
	opentelemetry.PluginBackup.Runs.Add(ctx, 1,
		metric.WithAttributes(
			opentelemetry.OutcomeFailure.Attribute(),
			opentelemetry.AttributeKeyFailureCategory.Of(category.Name),
		))
}

// recordVerificationSuccess records a successful verification.
func recordVerificationSuccess(ctx context.Context) {
	opentelemetry.PluginBackup.Verifications.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.OutcomeSuccess.Attribute()))
}

// recordVerificationFailure records a failed verification.
func recordVerificationFailure(ctx context.Context) {
	opentelemetry.PluginBackup.Verifications.Add(ctx, 1,
		metric.WithAttributes(opentelemetry.OutcomeFailure.Attribute()))
}

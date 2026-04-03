package cnpgi

import (
	"context"
	"time"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

//nolint:gochecknoinits
func init() {
	opentelemetry.InitBackupMetrics()
}

// recordBackupStart records that a backup has started.
func recordBackupStart(ctx context.Context) {
	opentelemetry.Backup.LatestStartTime.Record(ctx, float64(time.Now().Unix()))
	opentelemetry.Backup.Running.Record(ctx, 1)
}

// recordBackupSuccess records a successful backup completion.
func recordBackupSuccess(ctx context.Context, duration time.Duration) {
	opentelemetry.Backup.LatestCompletionTime.Record(ctx, float64(time.Now().Unix()))
	opentelemetry.Backup.LatestDuration.Record(ctx, duration.Seconds())
	opentelemetry.Backup.Running.Record(ctx, 0)
	opentelemetry.Backup.Successes.Add(ctx, 1)
}

// recordBackupFailure records a failed backup.
func recordBackupFailure(ctx context.Context) {
	opentelemetry.Backup.LatestFailureTime.Record(ctx, float64(time.Now().Unix()))
	opentelemetry.Backup.Running.Record(ctx, 0)
	opentelemetry.Backup.Failures.Add(ctx, 1)
}

// recordVerificationSuccess records a successful verification.
func recordVerificationSuccess(ctx context.Context) {
	opentelemetry.Backup.Verifications.Add(ctx, 1)
}

// recordVerificationFailure records a failed verification.
func recordVerificationFailure(ctx context.Context) {
	opentelemetry.Backup.Verifications.Add(ctx, 1)
	opentelemetry.Backup.VerificationFailures.Add(ctx, 1)
}

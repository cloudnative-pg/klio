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

// recordWalRestore records the end-to-end duration of one plugin WAL restore,
// tagged with outcome, cache_hit, tier and cluster_name. tier may be
// empty and cacheHit false when the restore failed before a tier served it.
func recordWalRestore(
	ctx context.Context,
	duration time.Duration,
	success, cacheHit bool,
	t tier,
	clusterName string,
) {
	outcome := opentelemetry.OutcomeSuccess
	if !success {
		outcome = opentelemetry.OutcomeFailure
	}

	opentelemetry.PluginWal.RestoreDuration.Record(ctx, duration.Nanoseconds(),
		metric.WithAttributes(
			outcome.Attribute(),
			opentelemetry.CacheHitOf(cacheHit).Attribute(),
			opentelemetry.AttributeKeyTier.Of(string(t)),
			opentelemetry.AttributeKeyClusterName.Of(clusterName),
		))
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

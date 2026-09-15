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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// clusterAttr returns the `cluster_name` attribute every plugin backup metric
// carries, so panels can attribute backup activity to a specific PostgreSQL
// cluster even when several clusters share one namespace.
func clusterAttr(clusterName string) attribute.KeyValue {
	return opentelemetry.AttributeKeyClusterName.Of(clusterName)
}

// recordBackupStart records that a backup has started. Callers must pair this
// with a deferred recordBackupFinished so the in-progress counter decrements
// on every exit path, including panics.
func recordBackupStart(ctx context.Context, clusterName string) {
	cluster := clusterAttr(clusterName)
	opentelemetry.PluginBackup.LatestStartTime.Record(ctx, time.Now().Unix(),
		metric.WithAttributes(cluster))
	opentelemetry.PluginBackup.InProgress.Add(ctx, 1, metric.WithAttributes(cluster))
}

// recordBackupFinished decrements the in-progress counter. Always invoke via
// defer immediately after recordBackupStart so concurrent backup accounting
// stays correct even when a backup panics or returns early. It must pass the
// same clusterName as recordBackupStart so the up/down counter cancels out per
// cluster.
func recordBackupFinished(ctx context.Context, clusterName string) {
	opentelemetry.PluginBackup.InProgress.Add(ctx, -1,
		metric.WithAttributes(clusterAttr(clusterName)))
}

// recordBackupSuccess records a successful backup completion.
func recordBackupSuccess(ctx context.Context, clusterName string, duration time.Duration) {
	cluster := clusterAttr(clusterName)
	opentelemetry.PluginBackup.LatestCompletionTime.Record(ctx, time.Now().Unix(),
		metric.WithAttributes(cluster))
	opentelemetry.PluginBackup.LatestDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(cluster))
	opentelemetry.PluginBackup.Duration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(cluster, opentelemetry.OutcomeSuccess.Attribute()))
	opentelemetry.PluginBackup.Runs.Add(ctx, 1,
		metric.WithAttributes(cluster, opentelemetry.OutcomeSuccess.Attribute()))
}

// recordBackupFailure records a failed backup.
func recordBackupFailure(ctx context.Context, clusterName string, duration time.Duration, err error) {
	cluster := clusterAttr(clusterName)
	category := classifyRunBackupError(ctx, err)
	opentelemetry.PluginBackup.LatestFailureTime.Record(ctx, time.Now().Unix(),
		metric.WithAttributes(cluster))
	opentelemetry.PluginBackup.Duration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(cluster, opentelemetry.OutcomeFailure.Attribute()))
	opentelemetry.PluginBackup.Runs.Add(ctx, 1,
		metric.WithAttributes(
			cluster,
			opentelemetry.OutcomeFailure.Attribute(),
			opentelemetry.AttributeKeyFailureCategory.Of(category.Name),
		))
}

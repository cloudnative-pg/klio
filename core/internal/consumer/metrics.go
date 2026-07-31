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

package consumer

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// recordRelay records the outcome of a tier2 relay attempt for a cluster. A
// failure means the backup did not reach tier2 on that attempt (the task is
// retried). The server emits this, so it is available even for clusters whose
// client has no plugin sidecar.
func recordRelay(ctx context.Context, clusterName string, err error) {
	opentelemetry.ServerBackup.Relay.Add(ctx, 1,
		metric.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(clusterName),
			outcomeOf(err).Attribute(),
		))
}

// recordMaintenance records the outcome of a maintenance run (base retention
// and WAL cleanup) for a cluster on a tier. tier1 maintenance is best-effort
// and tier2 WAL cleanup is best-effort, so for those this counter is the only
// signal that they failed.
func recordMaintenance(ctx context.Context, clusterName string, tier opentelemetry.Tier, err error) {
	opentelemetry.ServerBackup.Maintenance.Add(ctx, 1,
		metric.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(clusterName),
			tier.Attribute(),
			outcomeOf(err).Attribute(),
		))
}

func outcomeOf(err error) opentelemetry.Outcome {
	if err != nil {
		return opentelemetry.OutcomeFailure
	}

	return opentelemetry.OutcomeSuccess
}

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

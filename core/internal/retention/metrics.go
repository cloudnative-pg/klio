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

package retention

import (
	"context"

	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// recordMaintenance records the outcome of a sweep pass (base-snapshot
// retention and WAL cleanup) for a cluster on a tier.
func recordMaintenance(ctx context.Context, clusterName string, tier opentelemetry.Tier, err error) {
	outcome := opentelemetry.OutcomeSuccess
	if err != nil {
		outcome = opentelemetry.OutcomeFailure
	}

	opentelemetry.ServerBackup.Maintenance.Add(ctx, 1,
		metric.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(clusterName),
			tier.Attribute(),
			outcome.Attribute(),
		))
}

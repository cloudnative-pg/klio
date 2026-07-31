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

package repository

import (
	"context"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// Metrics holds OpenTelemetry metrics for the repository operations.
type Metrics struct {
	WalWrittenBytes       metric.Int64Counter
	WalWritten            metric.Int64Counter
	LatestWrittenTime     metric.Int64Gauge
	LatestWrittenLSN      metric.Int64Gauge
	LatestWrittenTimeline metric.Int64Gauge
	// BlockDuration is the per-block WAL stage duration histogram. It is only
	// set on the tier-1 server ingest path; the tier-2 consumer path leaves it
	// nil so that per-block observations are not emitted for tier-2.
	BlockDuration metric.Int64Histogram
	// Attributes are merged into every recording made through this Metrics.
	// Callers use this to attach tier="1" or tier="2" to the unified WAL
	// instruments depending on which side of the WAL pipeline they sit on.
	Attributes []attribute.KeyValue
}

// AttributeSet returns the canonical attribute.Set obtained by merging
// the per-recording extras onto the Metrics' base Attributes.
func (m *Metrics) AttributeSet(extra ...attribute.KeyValue) attribute.Set {
	return attribute.NewSet(slices.Concat(m.Attributes, extra)...)
}

// RecordBlockStage records a per-block WAL stage duration on the BlockDuration
// histogram. The storage tier is carried by the base Attributes; the cluster
// name, path, stage and outcome are added per call. The path distinguishes the
// ingest (put) and serve (get) flows, which share some stage names (e.g. send).
// It is a no-op when BlockDuration is nil, e.g. on the tier-2 consumer path
// which has no per-block histogram.
func (m *Metrics) RecordBlockStage(
	ctx context.Context,
	clusterName string,
	path opentelemetry.Path,
	stage opentelemetry.Stage,
	d time.Duration,
	outcome opentelemetry.Outcome,
) {
	if m.BlockDuration == nil {
		return
	}

	m.BlockDuration.Record(ctx, d.Nanoseconds(),
		metric.WithAttributeSet(m.AttributeSet(
			opentelemetry.AttributeKeyClusterName.Of(clusterName),
			path.Attribute(),
			stage.Attribute(),
			outcome.Attribute(),
		)),
	)
}

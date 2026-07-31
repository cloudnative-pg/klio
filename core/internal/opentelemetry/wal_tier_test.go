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

package opentelemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

// TestServerWalTierCollapse verifies that tier-1 (walserver) and tier-2
// (consumer) WAL recordings fold into a single instrument distinguished
// by the `tier` attribute, per CNP-8324.
func TestServerWalTierCollapse(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = provider.Shutdown(context.Background())
	})

	opentelemetry.InitServerWalMetrics()

	ctx := context.Background()
	opentelemetry.ServerWal.WalWritten.Add(ctx, 3,
		metric.WithAttributes(opentelemetry.Tier1.Attribute()))
	opentelemetry.ServerWal.WalWritten.Add(ctx, 5,
		metric.WithAttributes(opentelemetry.Tier2.Attribute()))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	var dps []metricdata.DataPoint[int64]
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != opentelemetry.ServerWalWrittenMetric {
				continue
			}
			s, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "klio.server.wal.written must be an Int64 Sum")
			dps = s.DataPoints
		}
	}

	require.Len(t, dps, 2,
		"expected unified klio.server.wal.written to expose one data point per tier")

	byTier := map[string]int64{}
	for _, dp := range dps {
		v, ok := dp.Attributes.Value(attribute.Key("tier"))
		require.True(t, ok, "every data point must carry a tier attribute")
		byTier[v.AsString()] = dp.Value
	}

	assert.Equal(t, int64(3), byTier[string(opentelemetry.Tier1)])
	assert.Equal(t, int64(5), byTier[string(opentelemetry.Tier2)])
}

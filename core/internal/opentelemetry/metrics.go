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

package opentelemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// walBlockDurationBuckets covers ~0.5ms to 1s for per-block stage timings.
func walBlockDurationBuckets() []float64 {
	return []float64{
		500_000, 1_000_000, 2_500_000, 5_000_000, 10_000_000, 25_000_000,
		50_000_000, 100_000_000, 250_000_000, 500_000_000, 1_000_000_000,
	}
}

// walFileDurationBuckets covers ~10ms to 60s for per-file operations.
func walFileDurationBuckets() []float64 {
	return []float64{
		10_000_000, 25_000_000, 50_000_000, 100_000_000, 250_000_000, 500_000_000,
		1_000_000_000, 2_500_000_000, 5_000_000_000, 10_000_000_000, 30_000_000_000, 60_000_000_000,
	}
}

// backupDurationBuckets spans 10s to two weeks (10s, 30s, 1m, 5m, 10m, 30m, 1h,
// 2h, 4h, 6h, 12h, 24h, 48h, 1w, 2w), hand-picked at meaningful calendar
// intervals.
func backupDurationBuckets() []float64 {
	return []float64{
		10, 30, 60, 300, 600, 1800, 3600, 7200, 14400, 21600, 43200, 86400, 172800, 604800, 1209600,
	}
}

// explicitBucketView returns a metric.View that applies the given explicit
// histogram bucket boundaries to the named instrument.
func explicitBucketView(instrumentName string, boundaries []float64) metric.View {
	return metric.NewView(
		metric.Instrument{Name: instrumentName},
		metric.Stream{
			Aggregation: metric.AggregationExplicitBucketHistogram{
				Boundaries: boundaries,
			},
		},
	)
}

// WalDurationViews returns the metric views that pin explicit bucket boundaries
// on the WAL duration histograms. They are applied by the MeterProvider and are
// exported so tests can build a provider with the same bucketing.
func WalDurationViews() []metric.View {
	return []metric.View{
		explicitBucketView(ServerWalBlockDurationMetric, walBlockDurationBuckets()),
		explicitBucketView(ClientWalBlockDurationMetric, walBlockDurationBuckets()),
		explicitBucketView(ServerWalGetDurationMetric, walFileDurationBuckets()),
		explicitBucketView(ServerWalUploadDurationMetric, walFileDurationBuckets()),
		explicitBucketView(PluginWalRestoreDurationMetric, walFileDurationBuckets()),
	}
}

// newMeterProvider creates a new OpenTelemetry MeterProvider with automatic resource detection support
// and integrates controller-runtime Prometheus metrics via a bridge.
func newMeterProvider(ctx context.Context) (*metric.MeterProvider, error) {
	res, err := createResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric reader: %w", err)
	}

	// Create a Prometheus bridge to collect metrics from the controller-runtime registry
	// This allows controller-runtime metrics to be exported through OTEL
	bridge := prometheus.NewMetricProducer(
		prometheus.WithGatherer(metrics.Registry),
	)

	// Create an OTLP exporter for the bridge that respects the same protocol configuration
	// Check OTEL_EXPORTER_OTLP_METRICS_PROTOCOL or fall back to OTEL_EXPORTER_OTLP_PROTOCOL
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if protocol == "" {
		protocol = "http/protobuf" // default
	}

	var bridgeExporter metric.Exporter
	if protocol == "grpc" {
		bridgeExporter, err = otlpmetricgrpc.New(ctx)
	} else {
		bridgeExporter, err = otlpmetrichttp.New(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter for bridge: %w", err)
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metricReader),
		// Create a periodic reader for the Prometheus bridge that exports to OTLP
		metric.WithReader(metric.NewPeriodicReader(
			bridgeExporter,
			metric.WithProducer(bridge),
		)),
		metric.WithResource(res),
		metric.WithView(explicitBucketView(PluginBackupDurationMetric, backupDurationBuckets())),
		metric.WithView(WalDurationViews()...),
	)

	return meterProvider, nil
}

// Package opentelemetry provides OpenTelemetry instrumentation for the operator.
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

// newMeterProvider creates a new OpenTelemetry MeterProvider with automatic
// resource detection support and integrates controller-runtime Prometheus
// metrics via a bridge.
func newMeterProvider(ctx context.Context) (*metric.MeterProvider, error) {
	res, err := buildResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build resource: %w", err)
	}

	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric reader: %w", err)
	}

	bridge := prometheus.NewMetricProducer(
		prometheus.WithGatherer(metrics.Registry),
	)

	// Check OTEL_EXPORTER_OTLP_METRICS_PROTOCOL or fall back to OTEL_EXPORTER_OTLP_PROTOCOL.
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
		metric.WithReader(metric.NewPeriodicReader(
			bridgeExporter,
			metric.WithProducer(bridge),
		)),
		metric.WithResource(res),
	)

	return meterProvider, nil
}

package opentelemetry

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	metricNoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	traceNoop "go.opentelemetry.io/otel/trace/noop"
)

const (
	// Meter is the name of the OpenTelemetry meter used by Klio.
	Meter = "klio"

	// resourceDetectorEnvVar is the environment variable used to specify resource detectors.
	resourceDetectorEnvVar = "OTEL_RESOURCE_DETECTORS"
)

// Init initializes OpenTelemetry, relies on `otel` global meter provider and
// on `otel` global tracer provider.
func Init(ctx context.Context) func() {
	contextLogger := log.FromContext(ctx)
	noop := func() {}

	if !isOtelConfigPresent() {
		contextLogger.Info("OpenTelemetry not configured, setting noop meter provider")
		otel.SetMeterProvider(metricNoop.NewMeterProvider())
		otel.SetTracerProvider(traceNoop.NewTracerProvider())

		return noop
	}

	otelMeterProvider, err := newMeterProvider(ctx)
	if err != nil {
		contextLogger.Error(err, "failed to setup otelMeterProvider")
		return noop
	}
	otel.SetMeterProvider(otelMeterProvider)

	otelTracerProvider, err := newTracerProvider(ctx)
	if err != nil {
		contextLogger.Error(err, "failed to setup otelTracerProvider")
		return noop
	}
	otel.SetTracerProvider(otelTracerProvider)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	if err := runtime.Start(runtime.WithMeterProvider(otelMeterProvider),
		runtime.WithMinimumReadMemStatsInterval(30*time.Second)); err != nil {
		contextLogger.Error(err, "failed to start runtime instrumentation")
	}
	if err := host.Start(host.WithMeterProvider(otelMeterProvider)); err != nil {
		contextLogger.Error(err, "failed to start host instrumentation")
	}

	return func() {
		if err := otelMeterProvider.Shutdown(ctx); err != nil {
			contextLogger.Error(err, "failed to shutdown otelMeterProvider")
		}

		if err := otelTracerProvider.Shutdown(ctx); err != nil {
			contextLogger.Error(err, "failed to shutdown otelTracerProvider")
		}
	}
}

func isOtelConfigPresent() bool {
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "OTEL_") {
			return true
		}
	}

	return false
}

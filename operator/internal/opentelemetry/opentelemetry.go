package opentelemetry

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	ctrl "sigs.k8s.io/controller-runtime"
)

var logger = ctrl.Log.WithName("opentelemetry") //nolint:gochecknoglobals

// Init initializes OpenTelemetry metrics for the operator. If no OTEL_*
// environment variables are set, a noop meter provider is installed.
// On setup failure it logs and falls back to a noop provider so that the
// operator keeps running without telemetry.
// The returned function flushes and shuts down the meter provider.
func Init(ctx context.Context) func(context.Context) error {
	noopShutdown := func(context.Context) error { return nil }

	if !isOtelConfigPresent() {
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return noopShutdown
	}

	provider, err := newMeterProvider(ctx)
	if err != nil {
		logger.Error(err, "failed to set up OpenTelemetry, continuing with noop provider")
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return noopShutdown
	}
	otel.SetMeterProvider(provider)

	if err := runtime.Start(
		runtime.WithMeterProvider(provider),
		runtime.WithMinimumReadMemStatsInterval(30*time.Second),
	); err != nil {
		logger.Error(err, "failed to start runtime instrumentation")
	}
	if err := host.Start(host.WithMeterProvider(provider)); err != nil {
		logger.Error(err, "failed to start host instrumentation")
	}

	return provider.Shutdown
}

func isOtelConfigPresent() bool {
	return slices.ContainsFunc(os.Environ(), func(env string) bool {
		return strings.HasPrefix(env, "OTEL_")
	})
}

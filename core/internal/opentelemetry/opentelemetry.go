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

	// Rebind the metric instruments to the real meter provider. They are first
	// created at package load time (in the catalog init function), when the
	// global meter provider is still the default delegating one. Synchronous
	// instruments keep working through that delegation, but observable
	// instruments must be recreated against the real provider: otherwise
	// RegisterCallback rejects them with "invalid observable: from a different
	// implementation" and their callbacks are never invoked, so no samples are
	// produced.
	InitPluginBackupMetrics()
	InitServerBackupMetrics()
	InitServerWalMetrics()

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

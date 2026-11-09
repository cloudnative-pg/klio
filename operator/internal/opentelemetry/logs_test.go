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
	"sync"
	"testing"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// memoryLogExporter collects the log records exported to it.
type memoryLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *memoryLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}

	return nil
}

func (*memoryLogExporter) Shutdown(context.Context) error { return nil }

func (*memoryLogExporter) ForceFlush(context.Context) error { return nil }

func (e *memoryLogExporter) withBody(body string) []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()

	var result []sdklog.Record
	for i := range e.records {
		if e.records[i].Body().AsString() == body {
			result = append(result, e.records[i])
		}
	}

	return result
}

//nolint:gochecknoglobals
var (
	memoryLogs = &memoryLogExporter{}

	// The autoexport registry panics on a duplicate name.
	registerMemoryLogs = sync.OnceFunc(func() {
		autoexport.RegisterLogExporter("klio-memory", func(context.Context) (sdklog.Exporter, error) {
			return memoryLogs, nil
		})
	})
)

func TestLoggingOptionsExportWithTheResourceOfTheMetrics(t *testing.T) {
	clearOTELEnv(t)
	registerMemoryLogs()
	memoryLogs.mu.Lock()
	memoryLogs.records = nil
	memoryLogs.mu.Unlock()
	t.Setenv("OTEL_LOGS_EXPORTER", "klio-memory")
	t.Setenv("OTEL_SERVICE_NAME", "klio-operator")
	t.Setenv("POD_NAME", "klio-controller-manager-0")

	options, err := LoggingOptions(t.Context())
	require.NoError(t, err)
	(&log.Flags{}).ConfigureLogging(options...)
	t.Cleanup(func() {
		loggerProvider = nil
		(&log.Flags{}).ConfigureLogging()
	})

	log.WithName("setup").Info("logs-marker")
	ShutdownLogs(t.Context())

	records := memoryLogs.withBody("logs-marker")
	require.Len(t, records, 1)
	assert.Equal(t, "setup", records[0].InstrumentationScope().Name)
	attributes := resourceAttrsToMap(records[0].Resource())
	assert.Equal(t, "klio-operator", attributes[string(semconv.ServiceNameKey)])
	assert.Equal(t, "klio-controller-manager-0", attributes[string(semconv.K8SPodNameKey)])
}

func TestLoggingOptionsWithoutExport(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "no OpenTelemetry configuration"},
		{
			name: "logs exporter not requested",
			env: map[string]string{
				"OTEL_SERVICE_NAME":           "klio-operator",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
			},
		},
		{
			name: "logs exporter disabled",
			env: map[string]string{
				"OTEL_LOGS_EXPORTER":          "none",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			options, err := LoggingOptions(t.Context())
			require.NoError(t, err)
			assert.Empty(t, options)
			assert.Nil(t, loggerProvider)
		})
	}
}

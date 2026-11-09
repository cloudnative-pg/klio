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
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// loggerProvider exports the log records, nil when the export is disabled.
//
//nolint:gochecknoglobals
var loggerProvider *sdklog.LoggerProvider

// LoggingOptions creates the provider exporting the log records, with the
// resource of the other signals and the exporter selected by
// OTEL_LOGS_EXPORTER, "otlp" when unset, and returns the options making
// ConfigureLogging use it. Like the other signals, it returns no option
// when OpenTelemetry is not configured or disabled, and when
// OTEL_LOGS_EXPORTER is "none". Call it before ConfigureLogging, so that
// every record is exported, and call ShutdownLogs before the process exits.
func LoggingOptions(ctx context.Context) ([]log.ConfigureOption, error) {
	if !isOtelConfigPresent() {
		return nil, nil
	}

	res, err := createResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	exp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}
	if autoexport.IsNoneLogExporter(exp) {
		return nil, nil
	}

	loggerProvider = sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)

	return []log.ConfigureOption{log.WithOpenTelemetryLoggerProvider(loggerProvider)}, nil
}

// ShutdownLogs exports the queued log records and stops their export,
// waiting at most five seconds for the collector, even when ctx is already
// cancelled. It is a no-op when the export is disabled.
func ShutdownLogs(ctx context.Context) {
	if loggerProvider == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := loggerProvider.Shutdown(ctx); err != nil {
		log.Error(err, "failed to shutdown the OpenTelemetry logger provider")
	}
}

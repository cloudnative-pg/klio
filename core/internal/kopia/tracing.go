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

package kopia

import "os"

// tracingEnvironmentVariables returns the environment variables Klio injects to
// enable Kopia's OTLP trace exporter, or nil when Klio should not touch the
// setting.
//
// Klio only provides a default. If the operator has explicitly set
// KOPIA_ENABLE_OTLP_TRACE in the environment (to "false" to disable Kopia
// tracing, or "true" to force it on), that value is left untouched and inherited
// by the Kopia subprocess. Otherwise Klio enables tracing only when its own
// traces are exported over OTLP/gRPC (see otlpTracesUseGRPC), reusing the
// standard OTEL_EXPORTER_OTLP_* variables (endpoint, TLS, headers, compression,
// timeout) already present in the current process, since Kopia can only export
// traces over gRPC.
func tracingEnvironmentVariables() []string {
	if _, explicit := os.LookupEnv("KOPIA_ENABLE_OTLP_TRACE"); explicit {
		return nil
	}

	if !otlpTracesUseGRPC() {
		return nil
	}

	return []string{"KOPIA_ENABLE_OTLP_TRACE=true"}
}

// otlpTracesExportEnabled reports whether Klio itself exports traces over OTLP.
// This mirrors the OTEL_TRACES_EXPORTER handling of autoexport (the package that
// drives Klio's own trace exporter): the value is a single exporter name that
// defaults to "otlp" when unset. Any other value ("none", "console", ...) means
// Klio is not exporting traces over OTLP, so Kopia must not either.
func otlpTracesExportEnabled() bool {
	exporter := os.Getenv("OTEL_TRACES_EXPORTER")
	return exporter == "" || exporter == "otlp"
}

// otlpTracesUseGRPC reports whether OTLP traces are configured for export over
// gRPC, which is the only protocol Kopia's trace exporter supports.
func otlpTracesUseGRPC() bool {
	if !otlpTracesExportEnabled() {
		return false
	}

	if os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" &&
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return false
	}

	// The signal-specific variable takes precedence over the generic one, as
	// per the OTLP specification.
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}

	return protocol == "grpc"
}

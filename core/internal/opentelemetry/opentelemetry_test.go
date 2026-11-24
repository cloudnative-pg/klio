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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsOtelConfigPresent(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{name: "no OTEL variables", expected: false},
		{
			name:     "exporter endpoint",
			env:      map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://otel-collector:4317"},
			expected: true,
		},
		{
			name:     "service name",
			env:      map[string]string{"OTEL_SERVICE_NAME": "klio-server"},
			expected: true,
		},
		{
			name:     "resource detectors",
			env:      map[string]string{"OTEL_RESOURCE_DETECTORS": "host"},
			expected: true,
		},
		{
			name: "SDK disabled",
			env: map[string]string{
				"OTEL_SDK_DISABLED":           "True",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "https://otel-collector:4317",
			},
			expected: false,
		},
		{
			name: "only Kubernetes service links",
			env: map[string]string{
				"OTEL_COLLECTOR_SERVICE_HOST":            "10.96.0.10",
				"OTEL_COLLECTOR_SERVICE_PORT":            "4317",
				"OTEL_COLLECTOR_PORT_4317_TCP":           "tcp://10.96.0.10:4317",
				"OTEL_COLLECTOR_MONITORING_SERVICE_HOST": "10.96.0.11",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearOTELEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			assert.Equal(t, tt.expected, isOtelConfigPresent())
		})
	}
}

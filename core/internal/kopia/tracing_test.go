package kopia

import (
	"slices"
	"testing"
)

func TestOtlpTracesUseGRPC(t *testing.T) {
	cases := []struct {
		name            string
		tracesExporter  string
		tracesEndpoint  string
		genericEndpoint string
		tracesProtocol  string
		genericProtocol string
		want            bool
	}{
		{
			name: "no endpoint configured",
			want: false,
		},
		{
			name:           "traces endpoint with traces protocol grpc",
			tracesEndpoint: "otel-collector:4317",
			tracesProtocol: "grpc",
			want:           true,
		},
		{
			name:            "generic endpoint with generic protocol grpc",
			genericEndpoint: "otel-collector:4317",
			genericProtocol: "grpc",
			want:            true,
		},
		{
			name:            "endpoint set but protocol http/protobuf",
			genericEndpoint: "otel-collector:4318",
			genericProtocol: "http/protobuf",
			want:            false,
		},
		{
			name:            "endpoint set but protocol unset",
			genericEndpoint: "otel-collector:4317",
			want:            false,
		},
		{
			name:            "traces protocol grpc takes precedence over generic http/protobuf",
			tracesEndpoint:  "otel-collector:4317",
			tracesProtocol:  "grpc",
			genericProtocol: "http/protobuf",
			want:            true,
		},
		{
			name:            "traces protocol http/protobuf takes precedence over generic grpc",
			genericEndpoint: "otel-collector:4317",
			tracesProtocol:  "http/protobuf",
			genericProtocol: "grpc",
			want:            false,
		},
		{
			name:           "traces exporter otlp with grpc",
			tracesExporter: "otlp",
			tracesEndpoint: "otel-collector:4317",
			tracesProtocol: "grpc",
			want:           true,
		},
		{
			name:           "traces exporter none disables even with grpc",
			tracesExporter: "none",
			tracesEndpoint: "otel-collector:4317",
			tracesProtocol: "grpc",
			want:           false,
		},
		{
			name:           "traces exporter console disables even with grpc",
			tracesExporter: "console",
			tracesEndpoint: "otel-collector:4317",
			tracesProtocol: "grpc",
			want:           false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_EXPORTER", tc.tracesExporter)
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", tc.tracesEndpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tc.genericEndpoint)
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", tc.tracesProtocol)
			t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tc.genericProtocol)

			if got := otlpTracesUseGRPC(); got != tc.want {
				t.Errorf("otlpTracesUseGRPC() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTracingEnvironmentVariables(t *testing.T) {
	const flag = "KOPIA_ENABLE_OTLP_TRACE=true"

	t.Run("enabled and propagated for grpc traces", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "otel-collector:4317")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "grpc")

		if got := tracingEnvironmentVariables(); !slices.Contains(got, flag) {
			t.Errorf("tracingEnvironmentVariables() = %v, want to contain %q", got, flag)
		}

		client := Client{}
		if got := client.kopiaEnvironmentVariables(); !slices.Contains(got, flag) {
			t.Errorf("kopiaEnvironmentVariables() = %v, want to contain %q", got, flag)
		}
	})

	t.Run("disabled without grpc traces", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")

		if got := tracingEnvironmentVariables(); got != nil {
			t.Errorf("tracingEnvironmentVariables() = %v, want nil", got)
		}

		client := Client{}
		if slices.Contains(client.kopiaEnvironmentVariables(), flag) {
			t.Errorf("kopiaEnvironmentVariables() unexpectedly contains %q", flag)
		}
	})

	t.Run("explicit false disables even for grpc traces", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "otel-collector:4317")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "grpc")
		t.Setenv("KOPIA_ENABLE_OTLP_TRACE", "false")

		if got := tracingEnvironmentVariables(); got != nil {
			t.Errorf("tracingEnvironmentVariables() = %v, want nil", got)
		}

		// Klio must not append its own =true; the operator's explicit value
		// stays in effect.
		client := Client{}
		if slices.Contains(client.kopiaEnvironmentVariables(), flag) {
			t.Errorf("kopiaEnvironmentVariables() unexpectedly contains %q", flag)
		}
	})

	t.Run("explicit true respected without Klio re-adding it", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")
		t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
		t.Setenv("KOPIA_ENABLE_OTLP_TRACE", "true")

		if got := tracingEnvironmentVariables(); got != nil {
			t.Errorf("tracingEnvironmentVariables() = %v, want nil", got)
		}

		// The operator's explicit =true is inherited from the environment.
		client := Client{}
		if got := client.kopiaEnvironmentVariables(); !slices.Contains(got, flag) {
			t.Errorf("kopiaEnvironmentVariables() = %v, want to contain %q", got, flag)
		}
	})
}

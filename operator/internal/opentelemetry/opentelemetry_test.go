package opentelemetry

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// clearOTELEnv removes all OTEL_* environment variables for the
// duration of the test, restoring them via t.Cleanup.
func clearOTELEnv(t *testing.T) {
	t.Helper()

	for _, env := range os.Environ() {
		if key, _, ok := strings.Cut(env, "="); ok && strings.HasPrefix(key, "OTEL_") {
			// t.Setenv saves the original value and restores it on cleanup.
			t.Setenv(key, os.Getenv(key))
			_ = os.Unsetenv(key)
		}
	}
}

func TestIsOtelConfigPresent(t *testing.T) {
	t.Run("returns false when no OTEL vars are set", func(t *testing.T) {
		clearOTELEnv(t)
		assert.False(t, isOtelConfigPresent())
	})

	t.Run("returns true when OTEL endpoint is set", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
		assert.True(t, isOtelConfigPresent())
	})

	t.Run("returns true for OTEL service name", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "klio-operator")
		assert.True(t, isOtelConfigPresent())
	})
}

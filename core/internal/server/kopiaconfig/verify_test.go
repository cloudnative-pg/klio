package kopiaconfig

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestVerifyKopiaRepository(t *testing.T) {
	t.Run("successful execution cleans up file", func(t *testing.T) {
		var capturedPath string
		tier := "tier1"

		err := verifyKopiaRepository(t.Context(), tier, func(tmpPath string) error {
			if !strings.Contains(tmpPath, "kopiaconfig_verify_"+tier) {
				t.Errorf("temp file path %s does not follow expected pattern", tmpPath)
			}

			capturedPath = tmpPath
			// Verify the file exists while inside the callback
			if _, statErr := os.Stat(tmpPath); os.IsNotExist(statErr) { //nolint:gosec
				t.Errorf("expected temp file %s to exist during callback", tmpPath)
			}

			return nil
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify the file is deleted after function returns
		if _, statErr := os.Stat(capturedPath); !os.IsNotExist(statErr) {
			t.Errorf("expected temp file %s to be deleted, but it still exists", capturedPath)
		}
	})

	t.Run("handles callback error and still cleans up", func(t *testing.T) {
		var capturedPath string
		tier := "tier1"
		expectedErrMsg := "mock callback failure"

		err := verifyKopiaRepository(t.Context(), "tier1", func(tmpPath string) error {
			if !strings.Contains(tmpPath, "kopiaconfig_verify_"+tier) {
				t.Errorf("temp file path %s does not follow expected pattern", tmpPath)
			}
			capturedPath = tmpPath

			return errors.New(expectedErrMsg)
		})

		// Check if the error is wrapped correctly
		if err == nil || !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("expected wrapped error containing '%s', got: %v", expectedErrMsg, err)
		}

		// Verify the file is still deleted despite the error
		if _, statErr := os.Stat(capturedPath); !os.IsNotExist(statErr) {
			t.Errorf("expected temp file %s to be deleted after error, but it still exists", capturedPath)
		}
	})
}

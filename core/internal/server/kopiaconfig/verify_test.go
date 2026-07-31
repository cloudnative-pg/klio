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
			if _, statErr := os.Stat(tmpPath); os.IsNotExist(statErr) {
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

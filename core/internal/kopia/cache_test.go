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

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupCacheDirectory(t *testing.T) {
	t.Run("SuccessfullyRemoveExistingDirectory", func(t *testing.T) {
		// Create a temporary directory structure for the test
		tempDir := t.TempDir()

		err := CleanupCacheDirectory(tempDir)
		if err != nil {
			t.Errorf("cleanupCache(%q) returned an unexpected error: %v", tempDir, err)
		}

		// Verify the directory is gone
		if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
			t.Errorf("expected directory %q to be removed, but os.Stat returned: %v", tempDir, err)
		}
	})

	t.Run("HandleNonExistentDirectory", func(t *testing.T) {
		// Create a path that we are certain does not exist
		nonExistentDir := filepath.Join(t.TempDir(), fmt.Sprintf("non-existent-cache-%d", os.Getpid()))

		err := CleanupCacheDirectory(nonExistentDir)
		if err != nil {
			// os.RemoveAll should return nil if the path does not exist, so cleanupCache should also return nil.
			t.Errorf("cleanupCache(%q) returned an unexpected error for non-existent path: %v",
				nonExistentDir, err)
		}
	})
}

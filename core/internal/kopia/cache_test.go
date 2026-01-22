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

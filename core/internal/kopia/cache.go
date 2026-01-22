package kopia

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanupCacheDirectory removes the cache directory at the specified path.
func CleanupCacheDirectory(name string) error {
	cleanPath := filepath.Clean(name)

	if !filepath.IsAbs(cleanPath) {
		return fmt.Errorf("cache directory %q must be an absolute path", cleanPath)
	}

	if cleanPath == string(filepath.Separator) {
		return fmt.Errorf("cannot clean root directory %q", cleanPath)
	}

	if err := os.RemoveAll(cleanPath); err != nil {
		return fmt.Errorf("while cleaning up cache directory %q: %w", cleanPath, err)
	}

	return nil
}

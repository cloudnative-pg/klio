package kopiaserver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func cleanupCache(name string) error {
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

func cacheDirectory(baseDir, subDir string) (string, error) {
	if baseDir == "" {
		return "", errors.New("cache base directory is empty")
	}

	joined := filepath.Join(baseDir, subDir)
	absolute, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("while resolving cache directory %q: %w", joined, err)
	}

	return absolute, nil
}

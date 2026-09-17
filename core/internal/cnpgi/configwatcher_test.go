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

package cnpgi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testConfigFileName = "config.yaml"

func TestConfigFileWatcherDetectsChange(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, testConfigFileName)

	require.NoError(t, os.WriteFile(configFile, []byte("initial content"), 0o600))

	watcher := NewConfigFileWatcher(configFile, 50*time.Millisecond)

	// Write new content after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(configFile, []byte("changed content"), 0o600)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := watcher(ctx)
	assert.ErrorIs(t, err, ErrConfigFileChanged)
}

func TestConfigFileWatcherNoChangeNoError(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, testConfigFileName)

	require.NoError(t, os.WriteFile(configFile, []byte("stable content"), 0o600))

	watcher := NewConfigFileWatcher(configFile, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := watcher(ctx)
	assert.NoError(t, err)
}

func TestConfigFileWatcherContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, testConfigFileName)

	require.NoError(t, os.WriteFile(configFile, []byte("content"), 0o600))

	watcher := NewConfigFileWatcher(configFile, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := watcher(ctx)
	assert.NoError(t, err)
}

func TestConfigFileWatcherInitialReadFailure(t *testing.T) {
	// Test that the watcher returns an error if the config file doesn't exist at startup
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.yaml")

	watcher := NewConfigFileWatcher(nonExistentFile, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := watcher(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "while reading initial config")
}

func TestConfigFileWatcherTransientReadError(t *testing.T) {
	// Test that transient read errors during polling don't cause immediate failure
	// The watcher should log the error and continue polling
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, testConfigFileName)

	require.NoError(t, os.WriteFile(configFile, []byte("initial content"), 0o600))

	watcher := NewConfigFileWatcher(configFile, 50*time.Millisecond)

	// Start the watcher in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Make the file temporarily unreadable (simulating transient unavailability)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.Chmod(configFile, 0o000)
		time.Sleep(100 * time.Millisecond)
		_ = os.Chmod(configFile, 0o600)
	}()

	// The watcher should not fail due to transient read errors
	err := watcher(ctx)
	assert.NoError(t, err) // Context timeout, not a file error
}

func TestHashFile(t *testing.T) {
	t.Run("returns consistent hash for same content", func(t *testing.T) {
		tmpDir := t.TempDir()
		file1 := filepath.Join(tmpDir, "file1.yaml")
		file2 := filepath.Join(tmpDir, "file2.yaml")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(file1, content, 0o600))
		require.NoError(t, os.WriteFile(file2, content, 0o600))

		hash1, err1 := hashFile(file1)
		hash2, err2 := hashFile(file2)

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("returns different hash for different content", func(t *testing.T) {
		tmpDir := t.TempDir()
		file1 := filepath.Join(tmpDir, "file1.yaml")
		file2 := filepath.Join(tmpDir, "file2.yaml")

		require.NoError(t, os.WriteFile(file1, []byte("content A"), 0o600))
		require.NoError(t, os.WriteFile(file2, []byte("content B"), 0o600))

		hash1, err1 := hashFile(file1)
		hash2, err2 := hashFile(file2)

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := hashFile("/nonexistent/path/file.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "while reading file")
	})

	t.Run("handles empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty.yaml")

		require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0o600))

		hash, err := hashFile(emptyFile)
		require.NoError(t, err)
		assert.NotEmpty(t, hash) // SHA256 of empty string is a valid hash
	})
}

// writeProjectedSecret lays out a directory the way the kubelet does for a
// projected Secret volume: a timestamped data directory, a `..data` symlink
// pointing at it, and one symlink per key at the top level.
func writeProjectedSecret(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	dataDir := filepath.Join(dir, "..2026_09_16_00_00_00.000000000")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dataDir, name), []byte(content), 0o600))
		_ = os.Remove(filepath.Join(dir, name))
		require.NoError(t, os.Symlink(filepath.Join("..data", name), filepath.Join(dir, name)))
	}
	_ = os.Remove(filepath.Join(dir, "..data"))
	require.NoError(t, os.Symlink(filepath.Base(dataDir), filepath.Join(dir, "..data")))
}

func TestHashPathDirectory(t *testing.T) {
	t.Run("follows the projected secret layout", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectedSecret(t, dir, map[string]string{
			"klio-archive": "archive",
			"source":       "recovery source",
		})

		hash, err := hashPath(dir)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
	})

	t.Run("changes when a sibling file changes", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectedSecret(t, dir, map[string]string{
			"klio-archive": "archive",
			"source":       "recovery source",
		})
		before, err := hashPath(dir)
		require.NoError(t, err)

		writeProjectedSecret(t, dir, map[string]string{
			"klio-archive": "archive",
			"source":       "rotated recovery source",
		})
		after, err := hashPath(dir)
		require.NoError(t, err)

		assert.NotEqual(t, before, after)
	})

	t.Run("changes when a file is renamed", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a"), []byte("same"), 0o600))
		before, err := hashPath(dir)
		require.NoError(t, err)

		require.NoError(t, os.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "b")))
		after, err := hashPath(dir)
		require.NoError(t, err)

		assert.NotEqual(t, before, after)
	})

	t.Run("is stable across polls", func(t *testing.T) {
		dir := t.TempDir()
		writeProjectedSecret(t, dir, map[string]string{"klio-archive": "archive"})

		first, err := hashPath(dir)
		require.NoError(t, err)
		second, err := hashPath(dir)
		require.NoError(t, err)

		assert.Equal(t, first, second)
	})

	t.Run("hashes a plain file like hashFile", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), testConfigFileName)
		require.NoError(t, os.WriteFile(file, []byte("content"), 0o600))

		fromPath, err := hashPath(file)
		require.NoError(t, err)
		fromFile, err := hashFile(file)
		require.NoError(t, err)

		assert.Equal(t, fromFile, fromPath)
	})
}

func TestConfigFileWatcherDetectsSiblingChange(t *testing.T) {
	dir := t.TempDir()
	writeProjectedSecret(t, dir, map[string]string{
		"klio-archive": "archive",
		"source":       "recovery source",
	})

	watcher := NewConfigFileWatcher(dir, 50*time.Millisecond)

	go func() {
		time.Sleep(100 * time.Millisecond)
		// Rewrite the file through its symlink, as a Secret rotation would.
		_ = os.WriteFile(filepath.Join(dir, "source"), []byte("rotated recovery source"), 0o600)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.ErrorIs(t, watcher(ctx), ErrConfigFileChanged)
}

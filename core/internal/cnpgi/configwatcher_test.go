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
	assert.Contains(t, err.Error(), "while reading initial config file")
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

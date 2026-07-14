package cnpgi

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextWALName(t *testing.T) {
	tests := []struct {
		name           string
		currentWAL     string
		segmentsPerLog uint64
		expectedNext   string
		expectError    bool
	}{
		{
			name:           "simple increment with 16MB segments",
			currentWAL:     "000000010000000000000001",
			segmentsPerLog: 256, // 4GB / 16MB
			expectedNext:   "000000010000000000000002",
		},
		{
			name:           "wrap segment to next log with 16MB segments",
			currentWAL:     "0000000100000000000000FF",
			segmentsPerLog: 256,
			expectedNext:   "000000010000000100000000",
		},
		{
			name:           "simple increment with 1GB segments",
			currentWAL:     "000000010000000000000001",
			segmentsPerLog: 4, // 4GB / 1GB
			expectedNext:   "000000010000000000000002",
		},
		{
			name:           "wrap segment to next log with 1GB segments",
			currentWAL:     "000000010000000000000003",
			segmentsPerLog: 4,
			expectedNext:   "000000010000000100000000",
		},
		{
			name:           "increment with timeline 2",
			currentWAL:     "000000020000000500000010",
			segmentsPerLog: 256,
			expectedNext:   "000000020000000500000011",
		},
		{
			name:           "wrap with high log number",
			currentWAL:     "00000001000000FF000000FF",
			segmentsPerLog: 256,
			expectedNext:   "000000010000010000000000",
		},
		{
			name:        "invalid WAL name length",
			currentWAL:  "00000001",
			expectError: true,
		},
		{
			name:        "invalid hex in timeline",
			currentWAL:  "GGGGGGGG0000000000000001",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &walPrefetcher{
				segmentsPerLog: tt.segmentsPerLog,
			}

			result, err := p.nextWALNameLocked(tt.currentWAL)

			if tt.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedNext, result)
		})
	}
}

func TestSpoolSubdir(t *testing.T) {
	tests := []struct {
		name     string
		opts     walRestoreOptions
		contains []string
	}{
		{
			name: "tier1 with simple config",
			opts: walRestoreOptions{
				configFile: "/var/lib/postgresql/klio/klio-archive",
				targetTier: tier1,
			},
			contains: []string{"tier1", "klio-archive"},
		},
		{
			name: "tier2 with different config",
			opts: walRestoreOptions{
				configFile: "/var/lib/postgresql/klio/source-cluster",
				targetTier: tier2,
			},
			contains: []string{"tier2", "source-cluster"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spoolSubdir(tt.opts)

			for _, s := range tt.contains {
				assert.Contains(t, result, s)
			}
		})
	}
}

func TestSpoolSubdirUniqueness(t *testing.T) {
	// Different config paths should produce different subdirectory names.
	opts1 := walRestoreOptions{
		configFile: "/var/lib/postgresql/klio/config1",
		targetTier: tier1,
	}
	opts2 := walRestoreOptions{
		configFile: "/var/lib/postgresql/klio/config2",
		targetTier: tier1,
	}

	result1 := spoolSubdir(opts1)
	result2 := spoolSubdir(opts2)

	assert.NotEqual(t, result1, result2)
}

func TestIsCompleteWALName(t *testing.T) {
	tests := []struct {
		name     string
		walName  string
		expected bool
	}{
		{
			name:     "valid complete WAL name",
			walName:  "000000010000000000000001",
			expected: true,
		},
		{
			name:     "valid complete WAL with high values",
			walName:  "00000002000000FF000000FF",
			expected: true,
		},
		{
			name:     "partial WAL file",
			walName:  "000000010000000000000001.partial",
			expected: false,
		},
		{
			name:     "history file",
			walName:  "00000002.history",
			expected: false,
		},
		{
			name:     "too short",
			walName:  "00000001",
			expected: false,
		},
		{
			name:     "too long",
			walName:  "0000000100000000000000010000",
			expected: false,
		},
		{
			name:     "empty string",
			walName:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCompleteWALName(tt.walName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewWALPrefetcher(t *testing.T) {
	t.Run("with default values", func(t *testing.T) {
		p := newWALPrefetcher("/tmp/spool", nil, 0, 0)
		defer func() { _ = p.Close() }()

		assert.Equal(t, "/tmp/spool", p.spoolDir)
		assert.Equal(t, defaultPrefetchCount, p.prefetchCount)
		assert.Equal(t, defaultDownloadTimeout, p.downloadTimeout)
		assert.NotNil(t, p.entries)
		assert.NotNil(t, p.downloadPool)
	})

	t.Run("with custom values", func(t *testing.T) {
		p := newWALPrefetcher("/custom/spool", nil, 5, 8)
		defer func() { _ = p.Close() }()

		assert.Equal(t, "/custom/spool", p.spoolDir)
		assert.Equal(t, 5, p.prefetchCount)
	})

	t.Run("negative prefetch count uses default", func(t *testing.T) {
		p := newWALPrefetcher("/tmp/spool", nil, -1, -1)
		defer func() { _ = p.Close() }()

		assert.Equal(t, defaultPrefetchCount, p.prefetchCount)
	})
}

func TestCleanupEntry(t *testing.T) {
	p := newWALPrefetcher("/tmp/spool", nil, 2, 2)
	defer func() { _ = p.Close() }()

	// Add some entries.
	p.mu.Lock()
	p.entries["000000010000000000000001"] = &walEntry{state: walStateReady}
	p.entries["000000010000000000000002"] = &walEntry{state: walStateReady}
	p.mu.Unlock()

	// Cleanup one entry.
	p.cleanupEntry("000000010000000000000001")

	p.mu.Lock()
	_, exists1 := p.entries["000000010000000000000001"]
	_, exists2 := p.entries["000000010000000000000002"]
	p.mu.Unlock()

	assert.False(t, exists1, "entry should be removed")
	assert.True(t, exists2, "other entry should remain")
}

func TestCleanupSpoolDir(t *testing.T) {
	t.Run("nonexistent directory returns nil", func(t *testing.T) {
		err := cleanupSpoolDir("/nonexistent/path/that/does/not/exist")
		assert.NoError(t, err)
	})

	t.Run("cleans up files in directory", func(t *testing.T) {
		// Create a temporary directory.
		tmpDir := t.TempDir()

		// Create some files.
		require.NoError(t, os.WriteFile(tmpDir+"/file1", []byte("test"), 0o600))
		require.NoError(t, os.WriteFile(tmpDir+"/file2", []byte("test"), 0o600))
		require.NoError(t, os.Mkdir(tmpDir+"/subdir", 0o750))
		require.NoError(t, os.WriteFile(tmpDir+"/subdir/file3", []byte("test"), 0o600))

		// Cleanup.
		err := cleanupSpoolDir(tmpDir)
		require.NoError(t, err)

		// Verify directory is empty.
		entries, err := os.ReadDir(tmpDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestLearnSegmentSize(t *testing.T) {
	t.Run("learns segment size from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		walFile := tmpDir + "/000000010000000000000001"

		// Create a file with known size (simulate 16MB WAL).
		data := make([]byte, 16*1024*1024)
		require.NoError(t, os.WriteFile(walFile, data, 0o600))

		p := newWALPrefetcher(tmpDir, nil, 2, 2)
		defer func() { _ = p.Close() }()

		p.learnSegmentSize(context.Background(), walFile)

		p.mu.Lock()
		segmentSize := p.segmentSize
		segmentsPerLog := p.segmentsPerLog
		p.mu.Unlock()

		assert.Equal(t, uint64(16*1024*1024), segmentSize)
		assert.Equal(t, uint64(256), segmentsPerLog) // 4GB / 16MB = 256
	})

	t.Run("does not overwrite existing segment size", func(t *testing.T) {
		tmpDir := t.TempDir()
		walFile := tmpDir + "/000000010000000000000001"

		// Create a file.
		data := make([]byte, 16*1024*1024)
		require.NoError(t, os.WriteFile(walFile, data, 0o600))

		p := newWALPrefetcher(tmpDir, nil, 2, 2)
		defer func() { _ = p.Close() }()

		// Pre-set segment size.
		p.mu.Lock()
		p.segmentSize = 1024 * 1024 // 1MB
		p.segmentsPerLog = 4096
		p.mu.Unlock()

		p.learnSegmentSize(context.Background(), walFile)

		p.mu.Lock()
		segmentSize := p.segmentSize
		p.mu.Unlock()

		// Should not have changed.
		assert.Equal(t, uint64(1024*1024), segmentSize)
	})

	t.Run("handles nonexistent file", func(t *testing.T) {
		p := newWALPrefetcher("/tmp", nil, 2, 2)
		defer func() { _ = p.Close() }()

		// Should not panic.
		p.learnSegmentSize(context.Background(), "/nonexistent/file")

		p.mu.Lock()
		segmentSize := p.segmentSize
		p.mu.Unlock()

		assert.Equal(t, uint64(0), segmentSize)
	})
}

func TestTriggerPrefetch(t *testing.T) {
	t.Run("does nothing when segmentsPerLog is zero", func(t *testing.T) {
		p := newWALPrefetcher("/tmp/spool", nil, 2, 2)
		defer func() { _ = p.Close() }()

		p.triggerPrefetch("000000010000000000000001")

		p.mu.Lock()
		entriesCount := len(p.entries)
		p.mu.Unlock()

		assert.Equal(t, 0, entriesCount)
	})

	t.Run("does not duplicate existing entries", func(t *testing.T) {
		p := newWALPrefetcher("/tmp/spool", nil, 2, 2)
		defer func() { _ = p.Close() }()

		p.mu.Lock()
		p.segmentSize = 16 * 1024 * 1024
		p.segmentsPerLog = 256
		// Pre-add the next WAL entry.
		p.entries["000000010000000000000002"] = &walEntry{state: walStateDownloading}
		p.mu.Unlock()

		p.triggerPrefetch("000000010000000000000001")

		p.mu.Lock()
		// Should have added 000000010000000000000003 but not duplicated 000000010000000000000002.
		_, exists2 := p.entries["000000010000000000000002"]
		_, exists3 := p.entries["000000010000000000000003"]
		entriesCount := len(p.entries)
		p.mu.Unlock()

		assert.True(t, exists2)
		assert.True(t, exists3)
		assert.Equal(t, 2, entriesCount)
	})
}

func TestWalState(t *testing.T) {
	// Verify state constants have expected values.
	assert.Equal(t, walStateDownloading, walState(0))
	assert.Equal(t, walStateReady, walState(1))
	assert.Equal(t, walStateFailed, walState(2))
}

// TestCanHavePartial verifies that only bare WAL segments are eligible for a
// .partial fallback: history and backup-label files (which carry an extension)
// must not trigger a nonsensical "<name>.partial" request.
func TestCanHavePartial(t *testing.T) {
	assert.True(t, canHavePartial("000000010000000000000001"))
	assert.False(t, canHavePartial("00000002.history"))
	assert.False(t, canHavePartial("000000010000000000000001.00000028.backup"))
	assert.False(t, canHavePartial("000000010000000000000001.partial"))
}

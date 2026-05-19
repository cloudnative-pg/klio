package klioclient

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestBackups returns a set of test backups.
func getTestBackups() (BackupMetadata, BackupMetadata, BackupMetadata) {
	firstBackup := BackupMetadata{
		Name:      "first-backup",
		StartedAt: time.Date(2025, 11, 7, 10, 10, 0, 0, time.UTC).Unix(),
		StoppedAt: time.Date(2025, 11, 7, 10, 15, 0, 0, time.UTC).Unix(),
	}
	secondBackup := BackupMetadata{
		Name:      "second-backup",
		StartedAt: time.Date(2025, 11, 7, 11, 10, 0, 0, time.UTC).Unix(),
		StoppedAt: time.Date(2025, 11, 7, 11, 15, 0, 0, time.UTC).Unix(),
	}
	thirdBackup := BackupMetadata{
		Name:      "third-backup",
		StartedAt: time.Date(2025, 11, 7, 12, 10, 0, 0, time.UTC).Unix(),
		StoppedAt: time.Date(2025, 11, 7, 12, 15, 0, 0, time.UTC).Unix(),
	}

	return firstBackup, secondBackup, thirdBackup
}

func TestSortByAscendingTime(t *testing.T) {
	firstBackup, secondBackup, thirdBackup := getTestBackups()

	t.Run("nil list", func(t *testing.T) {
		var nilList BackupList
		assert.NotPanics(t, func() { nilList.SortByAscendingTime() })
		assert.Nil(t, nilList)
	})

	t.Run("empty list", func(t *testing.T) {
		emptyList := BackupList{}
		assert.NotPanics(t, func() { emptyList.SortByAscendingTime() })
		assert.Empty(t, emptyList)
	})

	t.Run("single element", func(t *testing.T) {
		list := BackupList{firstBackup}
		list.SortByAscendingTime()
		require.Len(t, list, 1)
		assert.Equal(t, "first-backup", list[0].Name)
	})

	t.Run("already sorted list", func(t *testing.T) {
		list := BackupList{firstBackup, secondBackup, thirdBackup}
		list.SortByAscendingTime()
		require.Len(t, list, 3)
		assert.Equal(t, "first-backup", list[0].Name)
		assert.Equal(t, "second-backup", list[1].Name)
		assert.Equal(t, "third-backup", list[2].Name)
	})

	t.Run("reverse sorted list", func(t *testing.T) {
		list := BackupList{thirdBackup, secondBackup, firstBackup} // C, B, A
		list.SortByAscendingTime()
		require.Len(t, list, 3)
		assert.Equal(t, "first-backup", list[0].Name)
		assert.Equal(t, "second-backup", list[1].Name)
		assert.Equal(t, "third-backup", list[2].Name)
	})

	t.Run("unsorted list", func(t *testing.T) {
		list := BackupList{secondBackup, thirdBackup, firstBackup} // B, C, A
		list.SortByAscendingTime()
		require.Len(t, list, 3)
		assert.Equal(t, "first-backup", list[0].Name)
		assert.Equal(t, "second-backup", list[1].Name)
		assert.Equal(t, "third-backup", list[2].Name)
	})
}

func TestGetLatestBackup(t *testing.T) {
	firstBackup, secondBackup, thirdBackup := getTestBackups()

	t.Run("nil list", func(t *testing.T) {
		var nilList BackupList
		assert.Nil(t, nilList.GetLatestBackup())
	})

	t.Run("empty list", func(t *testing.T) {
		emptyList := BackupList{}
		assert.Nil(t, emptyList.GetLatestBackup())
	})

	t.Run("single element", func(t *testing.T) {
		list := BackupList{firstBackup}
		latest := list.GetLatestBackup()
		require.NotNil(t, latest)
		assert.Equal(t, "first-backup", latest.Name)
	})

	t.Run("multiple elements unsorted", func(t *testing.T) {
		// This list is unsorted to ensure GetLatestBackup sorts it correctly
		list := BackupList{secondBackup, thirdBackup, firstBackup}
		latest := list.GetLatestBackup()
		require.NotNil(t, latest)
		assert.Equal(t, "third-backup", latest.Name)
	})

	t.Run("multiple elements reverse sorted", func(t *testing.T) {
		// This list is reverse-sorted
		list := BackupList{thirdBackup, secondBackup, firstBackup}
		latest := list.GetLatestBackup()
		require.NotNil(t, latest)
		assert.Equal(t, "third-backup", latest.Name)
	})
}

func TestFindClosestBackup(t *testing.T) {
	firstBackup, secondBackup, _ := getTestBackups()

	t.Run("empty backup list", func(t *testing.T) {
		var emptyList BackupList
		assert.Nil(t, emptyList.FindClosestBackup(time.Now()))
	})

	t.Run("backup list with just a single element", func(t *testing.T) {
		singletonList := BackupList{
			firstBackup,
		}

		// There's no backup before the only backup we have
		assert.Nil(t, singletonList.FindClosestBackup(time.Date(2025, 11, 7, 8, 10, 0, 0, time.UTC)))

		// If we look exactly for the stop time, we get the only backup we have
		r := singletonList.FindClosestBackup(time.Date(2025, 11, 7, 10, 15, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "first-backup", r.Name)

		// After that, there's the only backup we have
		r = singletonList.FindClosestBackup(time.Date(2025, 11, 7, 10, 20, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "first-backup", r.Name)
	})

	t.Run("backup list with multiple elements", func(t *testing.T) {
		backupList := BackupList{
			firstBackup,
			secondBackup,
		}

		// There's no backup before the only backup we have
		assert.Nil(t, backupList.FindClosestBackup(time.Date(2025, 11, 7, 8, 10, 0, 0, time.UTC)))

		// If we look exactly for the stop time, of a backup, we get that backup
		r := backupList.FindClosestBackup(time.Date(2025, 11, 7, 10, 15, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "first-backup", r.Name)

		r = backupList.FindClosestBackup(time.Date(2025, 11, 7, 11, 15, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "second-backup", r.Name)

		// If we are after the latest backup, we get the last backup
		r = backupList.FindClosestBackup(time.Date(2025, 11, 7, 11, 20, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "second-backup", r.Name)

		// If our point in time is included in the second backup, we get the one immediately before
		r = backupList.FindClosestBackup(time.Date(2025, 11, 7, 11, 12, 0, 0, time.UTC))
		require.NotNil(t, r)
		assert.Equal(t, "first-backup", r.Name)

		// If our point in time is included in the first backup, we don't get anything
		r = backupList.FindClosestBackup(time.Date(2025, 11, 7, 10, 12, 0, 0, time.UTC))
		assert.Nil(t, r)
	})
}

// Mock Notifier.

type tablespaceCall struct {
	layout      TablespaceLayout
	destination string
}

// Mock Restorer.
type mockBackupRestorer struct {
	metadataToReturn *BackupMetadata
	listToReturn     BackupList
	errorToReturn    error

	getMetadataCalledWith string
	restorePgDataCalled   bool
	restoreControlCalled  bool
	tablespacesRestored   []tablespaceCall
}

// Implement the interface.
func (m *mockBackupRestorer) ListBackups(_ context.Context, _ string) (BackupList, error) {
	return m.listToReturn, m.errorToReturn
}

func (m *mockBackupRestorer) GetMetadata(_ context.Context, _ string, name string) (*BackupMetadata, error) {
	m.getMetadataCalledWith = name
	return m.metadataToReturn, m.errorToReturn
}

func (m *mockBackupRestorer) RestoreTablespace(
	_ context.Context,
	_ *BackupMetadata,
	tbl TablespaceLayout,
	destinationDirectory string,
) error {
	m.tablespacesRestored = append(m.tablespacesRestored, tablespaceCall{tbl, destinationDirectory})
	return m.errorToReturn
}

func (m *mockBackupRestorer) RestorePgData(_ context.Context, _ *BackupMetadata, _ string) error {
	m.restorePgDataCalled = true
	return m.errorToReturn
}

func (m *mockBackupRestorer) RestoreControlData(_ context.Context, _ *BackupMetadata, _ string) error {
	m.restoreControlCalled = true
	return m.errorToReturn
}

// Helper to create a new mock.
func newMockRestorer() *mockBackupRestorer {
	return &mockBackupRestorer{}
}

func TestNewRestoreExecutor(t *testing.T) {
	mock := newMockRestorer()
	conf := RestoreConfiguration{Name: "test-backup"}
	executor := NewRestoreExecutor(mock, conf)

	require.NotNil(t, executor)
	assert.Same(t, mock, executor.restorer)
	assert.Equal(t, conf, executor.configuration)
}

func TestRestoreExecutorRestore(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()
	mock := newMockRestorer()

	tablespaceOne := TablespaceLayout{Name: "tablespaceOne", Path: "/var/lib/tablespaceOne"}
	tablespaceTwo := TablespaceLayout{Name: "tablespaceTwo", Path: "/var/lib/tablespaceTwo"}

	meta := &BackupMetadata{
		Name:        "test-backup",
		BackupLabel: "test-label-content",
		Tablespaces: []TablespaceLayout{tablespaceOne, tablespaceTwo},
	}
	mock.metadataToReturn = meta

	conf := RestoreConfiguration{
		Name:            "test-backup",
		PgDataDirectory: tempDir,
		TablespacesDirectory: map[string]string{
			"tablespaceTwo": "/custom/path/tablespaceTwo",
		},
	}

	executor := NewRestoreExecutor(mock, conf)

	// Call restore
	err := executor.Restore(ctx, "", tempDir)

	require.NoError(t, err)

	// Check mock calls
	assert.Equal(t, "test-backup", mock.getMetadataCalledWith)
	assert.True(t, mock.restorePgDataCalled)
	assert.True(t, mock.restoreControlCalled)

	// Check tablespace calls
	require.Len(t, mock.tablespacesRestored, 2)
	assert.Equal(t, tablespaceOne, mock.tablespacesRestored[0].layout)
	assert.Equal(t, tablespaceOne.Path, mock.tablespacesRestored[0].destination) // Default path
	assert.Equal(t, tablespaceTwo, mock.tablespacesRestored[1].layout)
	assert.Equal(t, "/custom/path/tablespaceTwo", mock.tablespacesRestored[1].destination) // Custom path

	// Check filesystem
	// backup_label
	labelPath := path.Join(tempDir, backupLabelFileName)
	content, err := os.ReadFile(labelPath) //nolint:gosec // G304: File path is constructed from t.TempDir() in a test.
	require.NoError(t, err, "backup_label should be created")
	assert.Equal(t, "test-label-content", string(content))

	// pg_wal
	pgWalPath := path.Join(tempDir, "pg_wal")
	stat, err := os.Stat(pgWalPath)
	require.NoError(t, err, "pg_wal directory should be created")
	assert.True(t, stat.IsDir())
}

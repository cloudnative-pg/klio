package kopia

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// MockKlioClient is a mock implementation of klioclient.Client.

type MockKlioClient struct {
	klioclient.Client // Embed the interface to satisfy all methods

	ListBackupsFunc func(ctx context.Context, hostname string) (
		klioclient.BackupList, error)
	GetMetadataFunc func(ctx context.Context, hostname string,
		name string) (
		*klioclient.BackupMetadata, error)
	DeleteBackupFunc       func(ctx context.Context, hostname string, name string) error
	SetRetentionPolicyFunc func(ctx context.Context, t klioclient.Target,
		p klioclient.RetentionPolicy) error
	GetRetentionPolicyFunc func(ctx context.Context, t klioclient.Target) (
		*klioclient.RetentionPolicy, error)
	ApplyRetentionPolicyFunc func(ctx context.Context, t klioclient.Target) error
	UploadTablespaceFunc     func(ctx context.Context, backupName string,
		tbl klioclient.TablespaceLayout) error
	UploadPgDataFunc func(ctx context.Context, backupName string,
		pgData string) error
	UploadControlFileFunc func(ctx context.Context, backupName string,
		controlDataFileName string) error
	UploadBackupMetadataFunc func(ctx context.Context, backupName string,
		metadata *klioclient.BackupMetadata) error
	RestoreTablespaceFunc func(ctx context.Context,
		metadata *klioclient.BackupMetadata, tbl klioclient.TablespaceLayout,
		destinationDirectory string) error
	RestorePgDataFunc func(ctx context.Context,
		metadata *klioclient.BackupMetadata, destinationDirectory string) error
	RestoreControlDataFunc func(ctx context.Context,
		metadata *klioclient.BackupMetadata, destinationPath string) error
	GetUsernameFunc func() string
	GetHostnameFunc func() string
}

// Ensure MockKlioClient implements klioclient.Client.

var _ klioclient.Client = &MockKlioClient{}

func (m *MockKlioClient) ListBackups(ctx context.Context, hostname string) (klioclient.BackupList, error) {
	if m.ListBackupsFunc != nil {
		return m.ListBackupsFunc(ctx, hostname)
	}

	return nil, nil // Default empty list
}

func (m *MockKlioClient) GetMetadata(
	ctx context.Context, hostname string, name string,
) (*klioclient.BackupMetadata, error) {
	if m.GetMetadataFunc != nil {
		return m.GetMetadataFunc(ctx, hostname, name)
	}

	return nil, nil
}

func (m *MockKlioClient) DeleteBackup(ctx context.Context, hostname string, name string) error {
	if m.DeleteBackupFunc != nil {
		return m.DeleteBackupFunc(ctx, hostname, name)
	}

	return nil
}

func (m *MockKlioClient) SetRetentionPolicy(
	ctx context.Context, t klioclient.Target, p klioclient.RetentionPolicy,
) error {
	if m.SetRetentionPolicyFunc != nil {
		return m.SetRetentionPolicyFunc(ctx, t, p)
	}

	return nil
}

func (m *MockKlioClient) GetRetentionPolicy(
	ctx context.Context, t klioclient.Target,
) (*klioclient.RetentionPolicy, error) {
	if m.GetRetentionPolicyFunc != nil {
		return m.GetRetentionPolicyFunc(ctx, t)
	}

	return nil, nil
}

func (m *MockKlioClient) ApplyRetentionPolicy(ctx context.Context, t klioclient.Target) error {
	if m.ApplyRetentionPolicyFunc != nil {
		return m.ApplyRetentionPolicyFunc(ctx, t)
	}

	return nil
}

func (m *MockKlioClient) UploadTablespace(
	ctx context.Context, backupName string, tbl klioclient.TablespaceLayout,
) error {
	if m.UploadTablespaceFunc != nil {
		return m.UploadTablespaceFunc(ctx, backupName, tbl)
	}

	return nil
}

func (m *MockKlioClient) UploadPgData(ctx context.Context, backupName string, pgData string) error {
	if m.UploadPgDataFunc != nil {
		return m.UploadPgDataFunc(ctx, backupName, pgData)
	}

	return nil
}

func (m *MockKlioClient) UploadControlFile(ctx context.Context, backupName string, controlDataFileName string) error {
	if m.UploadControlFileFunc != nil {
		return m.UploadControlFileFunc(ctx, backupName, controlDataFileName)
	}

	return nil
}

func (m *MockKlioClient) UploadBackupMetadata(
	ctx context.Context, backupName string, metadata *klioclient.BackupMetadata,
) error {
	if m.UploadBackupMetadataFunc != nil {
		return m.UploadBackupMetadataFunc(ctx, backupName, metadata)
	}

	return nil
}

func (m *MockKlioClient) RestoreTablespace(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	tbl klioclient.TablespaceLayout,
	destinationDirectory string,
) error {
	if m.RestoreTablespaceFunc != nil {
		return m.RestoreTablespaceFunc(ctx, metadata, tbl, destinationDirectory)
	}

	return nil
}

func (m *MockKlioClient) RestorePgData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationDirectory string,
) error {
	if m.RestorePgDataFunc != nil {
		return m.RestorePgDataFunc(ctx, metadata, destinationDirectory)
	}

	return nil
}

func (m *MockKlioClient) RestoreControlData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationPath string,
) error {
	if m.RestoreControlDataFunc != nil {
		return m.RestoreControlDataFunc(ctx, metadata, destinationPath)
	}

	return nil
}

func (m *MockKlioClient) GetUsername() string {
	if m.GetUsernameFunc != nil {
		return m.GetUsernameFunc()
	}

	return ""
}

func (m *MockKlioClient) GetHostname() string {
	if m.GetHostnameFunc != nil {
		return m.GetHostnameFunc()
	}

	return ""
}

// newMockKlioClient creates a new, empty MockKlioClient struct.
func newMockKlioClient() *MockKlioClient {
	return &MockKlioClient{}
}

// Helper function to create a basic MultiConnection for testing.
func newTestMultiConnection() *MultiConnection {
	return &MultiConnection{
		Tier1: newMockKlioClient(), // Use MockKlioClient for consistent interface implementation
		Tier2: newMockKlioClient(), // Use MockKlioClient for consistent interface implementation
	}
}

// Helper function to create a new, empty BackupMetadata struct.
func newEmptyBackupMetadata() *klioclient.BackupMetadata {
	return &klioclient.BackupMetadata{}
}

func TestMarkTier1(t *testing.T) {
	meta := newEmptyBackupMetadata()

	result := markTier1(meta)

	assert.Equal(t, presentAnnotationValue, result.Annotations[tier1AnnotationName])
	assert.Equal(t, meta, result, "Should return the same metadata pointer")
}

func TestMarkTier2(t *testing.T) {
	meta := newEmptyBackupMetadata()

	result := markTier2(meta)

	assert.Equal(t, presentAnnotationValue, result.Annotations[tier2AnnotationName])
	assert.Equal(t, meta, result, "Should return the same metadata pointer")
}

func TestGetClientFromMetadata_Tier1(t *testing.T) {
	conn := newTestMultiConnection()

	// No annotation, should return nil
	meta1 := newEmptyBackupMetadata()
	client1 := conn.getClientFromMetadata(meta1)
	assert.Nil(t, client1, "Should return nil if no annotation is present")

	// Explicit Tier1 annotation
	meta2 := newEmptyBackupMetadata()
	markTier1(meta2)
	client2 := conn.getClientFromMetadata(meta2)
	assert.Equal(t, conn.Tier1, client2, "Should select Tier1 if explicitly annotated as Tier1")
}

func TestGetClientFromMetadata_Tier2(t *testing.T) {
	conn := newTestMultiConnection()

	// Explicit Tier2 annotation
	meta := newEmptyBackupMetadata()
	markTier2(meta)

	client := conn.getClientFromMetadata(meta)

	assert.Equal(t, conn.Tier2, client, "Should select Tier2 if explicitly annotated as Tier2")
}

func TestGetClientFromMetadata_NilTier2(t *testing.T) {
	// Simulate Tier2 being optional and not connected
	conn := &MultiConnection{
		Tier1: newMockKlioClient(), // Use MockKlioClient for consistency
		Tier2: nil,
	}

	meta := newEmptyBackupMetadata()
	markTier2(meta)

	// If Tier2 is annotated but the connection is nil, it should return nil
	client := conn.getClientFromMetadata(meta)

	assert.Nil(t, client, "If Tier2 is annotated but the connection is nil, it should return nil")
}

func TestListBackups(t *testing.T) {
	ctx := context.Background()
	hostname := "test-hostname"

	// Helper to create a backup metadata for tests
	createBackupMeta := func(name string) klioclient.BackupMetadata {
		return klioclient.BackupMetadata{
			Name:        name,
			Annotations: make(map[string]string),
			StartedAt:   time.Now().Unix(), // Use StartedAt as int64 Unix timestamp
		}
	}

	tests := []struct {
		name             string
		tier1Backups     klioclient.BackupList
		tier2Backups     klioclient.BackupList
		expectedBackups  klioclient.BackupList
		expectError      bool
		tier2ClientIsNil bool  // New field to simulate nil Tier2
		tier1Error       error // Added for simulating errors from Tier1
		tier2Error       error // Added for simulating errors from Tier2
	}{
		{
			name:         "Only Tier1 backups",
			tier1Backups: klioclient.BackupList{createBackupMeta("backup1"), createBackupMeta("backup2")},
			tier2Backups: klioclient.BackupList{},
			expectedBackups: klioclient.BackupList{
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup1")
					markTier1(&meta)

					return meta
				}(),
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup2")
					markTier1(&meta)

					return meta
				}(),
			},
			expectError: false,
		},
		{
			name:         "Only Tier2 backups",
			tier1Backups: klioclient.BackupList{},
			tier2Backups: klioclient.BackupList{createBackupMeta("backup3"), createBackupMeta("backup4")},
			expectedBackups: klioclient.BackupList{
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup3")
					markTier2(&meta)

					return meta
				}(),
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup4")
					markTier2(&meta)

					return meta
				}(),
			},
			expectError: false,
		},
		{
			name: "Backups in both tiers, some common",
			tier1Backups: klioclient.BackupList{
				createBackupMeta("common-backup"),
				createBackupMeta("tier1-only"),
			},
			tier2Backups: klioclient.BackupList{
				createBackupMeta("common-backup"),
				createBackupMeta("tier2-only"),
			},
			expectedBackups: klioclient.BackupList{
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("common-backup")
					markTier1(&meta)
					markTier2(&meta)

					return meta
				}(),
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("tier1-only")
					markTier1(&meta)

					return meta
				}(),
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("tier2-only")
					markTier2(&meta)

					return meta
				}(),
			},
			expectError: false,
		},
		{
			name:             "Tier2 client is nil",
			tier1Backups:     klioclient.BackupList{createBackupMeta("backup5")},
			tier2Backups:     klioclient.BackupList{createBackupMeta("backup6")}, // Should be ignored
			tier2ClientIsNil: true,
			expectedBackups: klioclient.BackupList{
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup5")
					markTier1(&meta)

					return meta
				}(),
			},
			expectError: false,
		},
		// Error handling for Tier1 ListBackups
		{
			name:             "Error in Tier1 ListBackups",
			tier1Backups:     klioclient.BackupList{},
			tier2Backups:     klioclient.BackupList{},
			expectError:      true,
			tier2ClientIsNil: false,
			// Make ListBackupsFunc return an error
			tier1Error: assert.AnError,
		},
		// Error handling for Tier2 ListBackups when Tier1 succeeds
		{
			name:             "Error in Tier2 ListBackups",
			tier1Backups:     klioclient.BackupList{createBackupMeta("backup7")},
			tier2Backups:     klioclient.BackupList{},
			expectError:      true,
			tier2ClientIsNil: false,
			tier2Error:       assert.AnError,
			expectedBackups: klioclient.BackupList{
				func() klioclient.BackupMetadata {
					meta := createBackupMeta("backup7")
					markTier1(&meta)

					return meta
				}(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTier1 := newMockKlioClient()
			mockTier1.ListBackupsFunc = func(_ context.Context, _ string) (klioclient.BackupList, error) {
				return tt.tier1Backups, tt.tier1Error
			}

			var clientTier2 klioclient.Client // Declare as interface type
			if !tt.tier2ClientIsNil {
				mockTier2 := newMockKlioClient()
				mockTier2.ListBackupsFunc = func(_ context.Context, _ string) (klioclient.BackupList, error) {
					return tt.tier2Backups, tt.tier2Error
				}
				clientTier2 = mockTier2 // Assign mock to interface
			} else {
				clientTier2 = nil // Explicitly set the interface to nil
			}

			multiConn := &MultiConnection{
				Tier1: mockTier1,
				Tier2: clientTier2,
			}

			result, err := multiConn.ListBackups(ctx, hostname)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err) // Use require.NoError here

				// Use assert.ElementsMatch for unordered lists
				assert.ElementsMatch(t, tt.expectedBackups, result)

				// Additional checks for annotations
				for _, backup := range result {
					assertBackupAnnotations(t, backup, tt.tier1Backups, tt.tier2Backups, tt.tier2ClientIsNil, tt.tier2Error)
				}
			}
		})
	}
}

// assertBackupAnnotations checks if the backup has the expected tier annotations.
func assertBackupAnnotations(
	t *testing.T,
	backup klioclient.BackupMetadata,
	tier1Backups klioclient.BackupList,
	tier2Backups klioclient.BackupList,
	tier2ClientIsNil bool,
	tier2Error error,
) {
	t.Helper()
	expectedInTier1 := false
	for _, b1 := range tier1Backups {
		if b1.Name == backup.Name {
			expectedInTier1 = true
			break
		}
	}

	expectedInTier2 := false
	if !tier2ClientIsNil && tier2Error == nil { // Only check Tier2 if it's not nil and no error
		for _, b2 := range tier2Backups {
			if b2.Name == backup.Name {
				expectedInTier2 = true
				break
			}
		}
	}

	if expectedInTier1 {
		assert.Equal(t, presentAnnotationValue, backup.Annotations[tier1AnnotationName],
			"Backup %s should be marked as tier1", backup.Name)
	} else {
		assert.NotContains(t, backup.Annotations, tier1AnnotationName,
			"Backup %s should not be marked as tier1", backup.Name)
	}

	if expectedInTier2 {
		assert.Equal(t, presentAnnotationValue, backup.Annotations[tier2AnnotationName],
			"Backup %s should be marked as tier2", backup.Name)
	} else {
		assert.NotContains(t, backup.Annotations, tier2AnnotationName,
			"Backup %s should not be marked as tier2", backup.Name)
	}
}

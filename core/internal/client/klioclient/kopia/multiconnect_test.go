package kopia

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// Helper function to create a basic MultiConnection for testing.
func newTestMultiConnection() *MultiConnection {
	return &MultiConnection{
		Tier1: &Connection{},
		Tier2: &Connection{},
	}
}

// Helper function to create a new, empty BackupMetadata struct.
func newEmptyBackupMetadata() *klioclient.BackupMetadata {
	return &klioclient.BackupMetadata{}
}

// Helper function to create a new BackupList.
func newTestBackupList(count int) klioclient.BackupList {
	list := make(klioclient.BackupList, count)
	for i := range count {
		list[i] = klioclient.BackupMetadata{Name: fmt.Sprintf("backup-%d", i)}
	}

	return list
}

func TestSetTierAnnotation_NilAnnotations(t *testing.T) {
	meta := newEmptyBackupMetadata()

	// Annotations is initially nil
	assert.Nil(t, meta.Annotations, "Annotations should start as nil")

	setTierAnnotation(meta, tier1AnnotationValue)

	// Annotations map should be initialized and contain the value
	assert.NotNil(t, meta.Annotations, "Annotations map should be initialized")
	assert.Equal(t, tier1AnnotationValue, meta.Annotations[tierAnnotationName])
}

func TestSetTierAnnotation_ExistingAnnotations(t *testing.T) {
	meta := newEmptyBackupMetadata()
	meta.Annotations = map[string]string{"test": "toast"}

	setTierAnnotation(meta, tier2AnnotationValue)

	// Annotations map should contain the new value and the existing value
	assert.Equal(t, tier2AnnotationValue, meta.Annotations[tierAnnotationName])
	assert.Equal(t, "toast", meta.Annotations["test"])
}

func TestMarkTier1(t *testing.T) {
	meta := newEmptyBackupMetadata()

	result := markTier1(meta)

	assert.Equal(t, tier1AnnotationValue, result.Annotations[tierAnnotationName])
	assert.Equal(t, meta, result, "Should return the same metadata pointer")
}

func TestMarkTier2(t *testing.T) {
	meta := newEmptyBackupMetadata()

	result := markTier2(meta)

	assert.Equal(t, tier2AnnotationValue, result.Annotations[tierAnnotationName])
	assert.Equal(t, meta, result, "Should return the same metadata pointer")
}

func TestMarkTier1List(t *testing.T) {
	list := newTestBackupList(3)

	result := markTier1List(list)

	assert.Len(t, result, 3)
	for i := range result {
		assert.Equal(t, tier1AnnotationValue, result[i].Annotations[tierAnnotationName])
	}
	assert.Equal(t, list, result, "Should return the same list slice")
}

func TestMarkTier2List(t *testing.T) {
	list := newTestBackupList(2)

	result := markTier2List(list)

	assert.Len(t, result, 2)
	for i := range result {
		assert.Equal(t, tier2AnnotationValue, result[i].Annotations[tierAnnotationName])
	}
	assert.Equal(t, list, result, "Should return the same list slice")
}

func TestMarkTierList_Empty(t *testing.T) {
	list := newTestBackupList(0)

	result := markTier1List(list)

	assert.Empty(t, result)
}

func TestGetClientFromMetadata_Tier1(t *testing.T) {
	conn := newTestMultiConnection()

	// No annotation (default to Tier1)
	meta1 := newEmptyBackupMetadata()
	client1 := conn.getClientFromMetadata(meta1)
	assert.Equal(t, conn.Tier1, client1, "Should default to Tier1 if annotation is missing")

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
		Tier1: &Connection{},
		Tier2: nil,
	}

	meta := newEmptyBackupMetadata()
	markTier2(meta)

	// Even if annotated as T2, if T2 is nil, it must fallback to T1
	client := conn.getClientFromMetadata(meta)

	assert.Equal(t, conn.Tier1, client, "If Tier2 is annotated but the connection is nil, it should fallback to Tier1")
}

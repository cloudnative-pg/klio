package kopia

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

const (
	tier1AnnotationName    = "klio.io/tier1"
	tier2AnnotationName    = "klio.io/tier2"
	presentAnnotationValue = "present"
)

// MultiConnection is composed by two clients: one for tier1
// and one for tier2.
type MultiConnection struct {
	Tier1 klioclient.Client
	Tier2 klioclient.Client
}

// MultiConnect creates two connections: one to tier1 and one to
// tier2 (optional). Writes are directly routed to tier1. Reads
// are tried against tier1 and, if the required information has
// not been found, they are tried against tier2.
func MultiConnect(
	ctx context.Context,
	kopiaClientConfig *config.BaseRepositoryClientConfig,
) (*MultiConnection, error) {
	tier1, err := ConnectTier1(ctx, kopiaClientConfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to tier1: %w", err)
	}

	mc := &MultiConnection{
		Tier1: tier1,
	}

	if kopiaClientConfig.Tier2URL != "" {
		tier2, err := ConnectTier2(ctx, kopiaClientConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to tier2: %w", err)
		}
		mc.Tier2 = tier2
	}

	return mc, nil
}

// GetUsername implements the Client interface.
func (s *MultiConnection) GetUsername() string {
	return s.Tier1.GetUsername()
}

// GetHostname implements the Client interface.
func (s *MultiConnection) GetHostname() string {
	return s.Tier1.GetHostname()
}

// DeleteBackup implements the Client interface.
func (s *MultiConnection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	return s.Tier1.DeleteBackup(ctx, hostname, name)
}

// SetRetentionPolicy implements the Client interface.
func (s *MultiConnection) SetRetentionPolicy(
	ctx context.Context,
	t klioclient.Target,
	p klioclient.RetentionPolicy,
) error {
	return s.Tier1.SetRetentionPolicy(ctx, t, p)
}

// GetRetentionPolicy implements the Client interface.
func (s *MultiConnection) GetRetentionPolicy(
	ctx context.Context,
	t klioclient.Target,
) (*klioclient.RetentionPolicy, error) {
	return s.Tier1.GetRetentionPolicy(ctx, t)
}

// GetMetadata implements the BackupRestoreSupport interface.
func (s *MultiConnection) GetMetadata(
	ctx context.Context,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	meta, err := s.Tier1.GetMetadata(ctx, hostname, name)
	if err == nil {
		return markTier1(meta), nil
	}

	var noBackup NoBackupFoundError
	if !errors.As(err, &noBackup) {
		return nil, fmt.Errorf("while getting metadata from tier1: %w", err)
	}

	if s.Tier2 == nil {
		return nil, newNoBackupFoundError(hostname, name)
	}

	meta, err = s.Tier2.GetMetadata(ctx, hostname, name)
	if err != nil {
		return nil, fmt.Errorf("while getting metadata from tier2: %w", err)
	}

	return markTier2(meta), nil
}

// ApplyRetentionPolicy implements the Client interface.
func (s *MultiConnection) ApplyRetentionPolicy(ctx context.Context, t klioclient.Target) error {
	return s.Tier1.ApplyRetentionPolicy(ctx, t)
}

// ListBackups implements the BackupRestoreSupport interface.
func (s *MultiConnection) ListBackups(ctx context.Context, hostname string) (klioclient.BackupList, error) {
	var tier1List, tier2List klioclient.BackupList

	tier1List, err := s.Tier1.ListBackups(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("while listing backups from tier1: %w", err)
	}

	if s.Tier2 != nil {
		tier2List, err = s.Tier2.ListBackups(ctx, hostname)
		if err != nil {
			return nil, fmt.Errorf("while listing backups from tier2: %w", err)
		}
	}

	tier1BackupNames := stringset.New()
	for i := range tier1List {
		tier1BackupNames.Put(tier1List[i].Name)
	}

	tier2BackupNames := stringset.New()
	for i := range tier2List {
		tier2BackupNames.Put(tier2List[i].Name)
	}

	result := make(klioclient.BackupList, 0, len(tier1List)+len(tier2List))

	for i := range tier1List {
		markTier1(&tier1List[i])

		if tier2BackupNames.Has(tier1List[i].Name) {
			markTier2(&tier1List[i])
		}

		result = append(result, tier1List[i])
	}

	for i := range tier2List {
		markTier2(&tier2List[i])

		if tier1BackupNames.Has(tier2List[i].Name) {
			// This backup has been put in the backup list
			// by the previous loop
			continue
		}

		result = append(result, tier2List[i])
	}

	return result, nil
}

// RestoreTablespace implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestoreTablespace(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	tbl klioclient.TablespaceLayout,
	destinationDirectory string,
) error {
	return s.getClientFromMetadata(metadata).RestoreTablespace(ctx, metadata, tbl, destinationDirectory)
}

// RestorePgData implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestorePgData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationDirectory string,
) error {
	return s.getClientFromMetadata(metadata).RestorePgData(ctx, metadata, destinationDirectory)
}

// RestoreControlData implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestoreControlData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationPath string,
) error {
	return s.getClientFromMetadata(metadata).RestoreControlData(ctx, metadata, destinationPath)
}

// UploadTablespace implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadTablespace(
	ctx context.Context,
	backupName string,
	tbl klioclient.TablespaceLayout,
) error {
	return s.Tier1.UploadTablespace(ctx, backupName, tbl)
}

// UploadPgData implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadPgData(
	ctx context.Context,
	backupName string,
	pgData string,
) error {
	return s.Tier1.UploadPgData(ctx, backupName, pgData)
}

// UploadControlFile implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadControlFile(
	ctx context.Context,
	backupName string,
	controlDataFileName string,
) error {
	return s.Tier1.UploadControlFile(ctx, backupName, controlDataFileName)
}

// UploadBackupMetadata implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadBackupMetadata(
	ctx context.Context,
	backupName string,
	metadata *klioclient.BackupMetadata,
) error {
	return s.Tier1.UploadBackupMetadata(ctx, backupName, metadata)
}

func (s *MultiConnection) getClientFromMetadata(meta *klioclient.BackupMetadata) klioclient.Client { //nolint:ireturn
	if meta.Annotations[tier1AnnotationName] == presentAnnotationValue {
		return s.Tier1
	}

	if meta.Annotations[tier2AnnotationName] == presentAnnotationValue {
		return s.Tier2
	}

	return nil
}

func markTier1(meta *klioclient.BackupMetadata) *klioclient.BackupMetadata {
	meta.SetAnnotation(tier1AnnotationName, presentAnnotationValue)

	return meta
}

func markTier2(meta *klioclient.BackupMetadata) *klioclient.BackupMetadata {
	meta.SetAnnotation(tier2AnnotationName, presentAnnotationValue)

	return meta
}

package kopia

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// GetMetadata implements the RestoreExecutor interface.
func (s *Connection) GetMetadata(
	ctx context.Context,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName:    name,
		klioclient.BackupContentTagName: "metadata",
	})
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	for _, entry := range entries {
		if entry.Source.Host == hostname {
			return s.restoreMetadata(ctx, entry.ID)
		}
	}

	return nil, newNoBackupFoundError(hostname, name)
}

// ListBackups list all the backups in the repository.
func (s *Connection) ListBackups(ctx context.Context, hostname string) (klioclient.BackupList, error) {
	contextLogger := log.FromContext(ctx)

	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupContentTagName: "metadata",
	})
	if err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	result := make([]klioclient.BackupMetadata, 0, len(entries))
	for _, entry := range entries {
		if hostname != "" && entry.Source.Host != hostname {
			continue
		}

		metadata, err := s.restoreMetadata(ctx, entry.ID)
		if err != nil {
			contextLogger.Error(err, "Error while decoding backup metadata, skipping", "id", entry.ID)
		} else {
			result = append(result, *metadata)
		}
	}

	return result, nil
}

// restoreMetadata restores the metadata stored in a snapshot with the
// given ID.
func (s *Connection) restoreMetadata(
	ctx context.Context,
	snapshotID string,
) (*klioclient.BackupMetadata, error) {
	manifestContent, err := s.kopia.RestoreSingleFile(ctx, snapshotID, "metadata.json")
	if err != nil {
		return nil, fmt.Errorf("restoring backup metadata: %w", err)
	}

	var result klioclient.BackupMetadata
	if err := json.Unmarshal(manifestContent, &result); err != nil {
		return nil, fmt.Errorf("cannot decode JSON backup metadata: %w", err)
	}

	return &result, nil
}

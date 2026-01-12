package kopia

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// RestoreTablespace implements the RestoreExecutor interface.
func (s *Connection) RestoreTablespace(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	tbl klioclient.TablespaceLayout,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName:  "tablespace",
		klioclient.TablespaceNameTagName: tbl.Name,
		klioclient.BackupNameTagName:     metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.kopia.RestoreSnapshot(ctx, source, destinationDirectory)
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *Connection) RestorePgData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName: "pgdata",
		klioclient.BackupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.kopia.RestoreSnapshot(ctx, source, destinationDirectory)
}

// RestoreControlData restores the control data from the backup.
func (s *Connection) RestoreControlData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationPath string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName: "controldata",
		klioclient.BackupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.kopia.RestoreSnapshot(ctx, source, destinationPath)
}

func (s *Connection) getSnapshotID(
	ctx context.Context,
	tags map[string]string,
) (string, error) {
	entries, err := s.kopia.ListSnapshots(ctx, tags)
	if err != nil {
		return "", fmt.Errorf("while executing Kopia command: %w", err)
	}

	for _, entry := range entries {
		if s.GetHostname() != "" && entry.Source.Host == s.GetHostname() {
			return entry.ID, nil
		}
	}

	return "", newNoSnapshotFound(s.GetHostname(), tags)
}

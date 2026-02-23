package kopia

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// ErrBackupNotFound is returned when attempting to delete a backup that does not exist.
var ErrBackupNotFound = errors.New("backup not found")

// DeleteBackup removes all snapshots associated with the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	contextLogger := log.FromContext(ctx)

	// List all snapshots for this backup (all content types)
	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: name,
	}, contextLogger.Debug)
	if err != nil {
		return fmt.Errorf("while listing snapshots: %w", err)
	}

	var deleted int
	for _, entry := range entries {
		if entry.Source.Host == hostname {
			contextLogger.Info("DeleteBackup: deleting snapshot", "snapshotID", entry.ID)
			if deleteErr := s.kopia.DeleteSnapshot(ctx, entry.ID); deleteErr != nil {
				err = errors.Join(err, deleteErr)
			} else {
				deleted++
			}
		}
	}

	if err != nil {
		return err
	}

	if deleted == 0 {
		return fmt.Errorf("%w: %s", ErrBackupNotFound, name)
	}

	return nil
}

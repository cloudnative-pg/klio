package kopia

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/kopia/kopia/repo"
)

// DeleteBackup removes the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, name string) error {
	contextLogger := log.FromContext(ctx)

	// Look for the kopia manifest with that name
	entries, err := s.repository.FindManifests(ctx, map[string]string{
		backupNameTagName: name,
	})
	if err != nil {
		return fmt.Errorf("while looking for backup entry: %w", err)
	}
	if len(entries) > 1 {
		return newMultipleBackupsFoundError(name, len(entries))
	}
	if len(entries) == 0 {
		return newNoBackupFoundError(name)
	}

	ctx, writer, err := s.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("deleting backup %q for hostname %q", name, s.hostname),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}
	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			contextLogger.Error(err, "while closing repository write session to archive WALs")
		}
	}()

	if err := writer.DeleteManifest(ctx, entries[0].ID); err != nil {
		return fmt.Errorf("while deleting manifest %q: %w", entries[0].ID, err)
	}

	if err := writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

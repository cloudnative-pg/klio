package kopia

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// DeleteBackup removes the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	contextLogger := log.FromContext(ctx)
	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName:    name,
		klioclient.BackupContentTagName: "metadata",
	}, contextLogger.Debug)
	if err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	for _, entry := range entries {
		if entry.Source.Host == hostname {
			err = errors.Join(err, s.kopia.DeleteSnapshot(ctx, entry.ID))
		}
	}

	return err
}

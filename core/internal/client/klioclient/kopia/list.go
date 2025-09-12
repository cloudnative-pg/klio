package kopia

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// GetMetadata implements the RestoreExecutor interface.
func (s *RestoreImplementation) GetMetadata(ctx context.Context, name string) (*common.BackupMetadata, error) {
	// Look for the kopia manifest with that name
	labelsToMatch := map[string]string{
		backupNameTagName: name,
	}
	if s.hostname != "" {
		labelsToMatch[snapshot.HostnameLabel] = s.hostname
	}
	entries, err := s.repository.FindManifests(ctx, labelsToMatch)
	if err != nil {
		return nil, fmt.Errorf("while looking for backup entry: %w", err)
	}
	if len(entries) > 1 {
		return nil, newMultipleBackupsFoundError(name, len(entries))
	}
	if len(entries) == 0 {
		return nil, newNoBackupFoundError(name)
	}

	snapshotManifest, err := snapshot.LoadSnapshot(ctx, s.repository, entries[0].ID)
	if err != nil {
		return nil, fmt.Errorf("while loading snapshot from manifest ID %q: %w", entries[0].ID, err)
	}

	var metadata common.BackupMetadata
	if err := json.Unmarshal([]byte(snapshotManifest.Description), &metadata); err != nil {
		return nil, fmt.Errorf("while unmarshalling backup description for %q: %w", name, err)
	}

	if metadata.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}
	metadata.Annotations[pgDataManifestIDAnnotationName] = string(snapshotManifest.ID)

	return &metadata, nil
}

// ListBackups list all the backups in the repository.
func (s *RestoreImplementation) ListBackups(ctx context.Context) ([]common.BackupMetadata, error) {
	// Look for every kopia manifest, and filter for tags later
	labelsToMatch := map[string]string{
		manifest.TypeLabelKey: snapshot.ManifestType,
	}
	if s.hostname != "" {
		labelsToMatch[snapshot.HostnameLabel] = s.hostname
	}
	entries, err := s.repository.FindManifests(ctx, labelsToMatch)
	if err != nil {
		return nil, fmt.Errorf("while looking for backup entry: %w", err)
	}

	result := make([]common.BackupMetadata, 0, len(entries))

	for _, entry := range entries {
		snapshotManifest, err := snapshot.LoadSnapshot(ctx, s.repository, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("while loading snapshot from manifest ID %q: %w", entry.ID, err)
		}

		if snapshotManifest.Tags["content"] != "pgdata" {
			// SKIPPING
			continue
		}

		var metadata common.BackupMetadata
		if err := json.Unmarshal([]byte(snapshotManifest.Description), &metadata); err != nil {
			return nil, fmt.Errorf("while unmarshalling backup description for %q: %w", entry.ID, err)
		}

		if metadata.Annotations == nil {
			metadata.Annotations = make(map[string]string)
		}

		metadata.Annotations[pgDataManifestIDAnnotationName] = string(snapshotManifest.ID)
		result = append(result, metadata)
	}

	return result, nil
}

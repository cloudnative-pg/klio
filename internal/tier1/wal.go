package tier1

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

// StoreWAL stores a WAL file inside the repository
func (s *impl) StoreWAL(ctx context.Context, name string, content []byte) error {
	if s.repository == nil {
		return fmt.Errorf("repository is not initialized")
	}

	// This enables Kopia debugging
	// ctx = logging.WithLogger(ctx, logging.ToWriter(os.Stdout))
	ctx, writer, err := s.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("archiving %s", name),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}

	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			s.logger.Error("while closing repository write session to archive WALs", "err", err)
		}
	}()

	source := getWALFileEntry(name, content)
	sourceInfo := snapshot.SourceInfo{
		Host:     s.config.ClusterName,
		UserName: s.config.ClusterName,
		Path:     path.Join("/wal", name),
	}

	policyTree, err := policy.TreeForSource(ctx, s.repository, sourceInfo)
	if err != nil {
		return fmt.Errorf("while getting policy tree: %w", err)
	}

	uploader := snapshotfs.NewUploader(writer)
	manifest, err := uploader.Upload(ctx, source, policyTree, sourceInfo)
	if err != nil {
		return fmt.Errorf("while uploading WAL file to storage: %w", err)
	}
	manifest.Tags = map[string]string{
		"content": "wal",
	}

	manifestID, err := snapshot.SaveSnapshot(ctx, writer, manifest)
	if err != nil {
		return fmt.Errorf("while saving manifest ID to repository: %w", err)
	}

	err = writer.Flush(ctx)
	if err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	s.logger.Debug("Saved WAL file to tier1 storage", "manifestID", manifestID)
	return nil
}

// GetLatestWALFileName returns the latest WAL file that have been archived
func (s *impl) GetLatestWALFileName(ctx context.Context) (string, error) {
	if s.repository == nil {
		return "", fmt.Errorf("repository is not initialized")
	}

	// todo: this doesn't scale well, we need to find a better solution.
	manifests, err := s.repository.FindManifests(ctx, s.getSnapshotRepositoryLabels())
	if err != nil {
		return "", fmt.Errorf("while finding manifests: %w", err)
	}

	// check wal file name to determine the latest one instead of id
	snap, err := snapshot.LoadSnapshot(ctx, s.repository, manifest.PickLatestID(manifests))
	if err != nil {
		return "", fmt.Errorf("while loading snapshot: %w", err)
	}

	return filepath.Base(snap.Source.Path), nil
}

func (s *impl) GetWAL(ctx context.Context, walName string) (*WalEntry, error) {
	if s.repository == nil {
		return nil, fmt.Errorf("repository is not initialized")
	}

	manifests, err := s.repository.FindManifests(ctx, s.getSnapshotRepositoryLabels())
	if err != nil {
		return nil, fmt.Errorf("while finding manifests: %w", err)
	}

	var walManifest *manifest.EntryMetadata
	for _, m := range manifests {
		if filepath.Base(m.Labels["path"]) == walName {
			walManifest = m
			break
		}
	}
	if walManifest == nil {
		return nil, fmt.Errorf("WAL file not found: %s", walName)
	}

	mf, err := snapshot.LoadSnapshot(ctx, s.repository, walManifest.ID)
	if err != nil {
		return nil, fmt.Errorf("while loading snapshot")
	}

	reader, err := s.repository.OpenObject(ctx, mf.RootObjectID())
	if err != nil {
		return nil, fmt.Errorf("while opening object: %w", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			s.logger.Error("while closing reader", "err", err)
		}
	}()

	readerData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("while reading object: %w", err)
	}

	return &WalEntry{walName: walName, content: readerData}, nil
}

func (s *impl) getSnapshotRepositoryLabels() map[string]string {
	return map[string]string{
		"type":     "snapshot",
		"hostname": s.config.ClusterName,
		"username": s.config.ClusterName,
		"content":  "wal",
	}
}

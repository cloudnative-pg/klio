package tier1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

// WALNotFoundError is returned when the WAL file is not found.
type WALNotFoundError struct {
	walName string
}

func (e *WALNotFoundError) Error() string {
	return fmt.Sprintf("WAL file not found: %s", e.walName)
}

// RepositoryNotInitializedError is returned when the repository is not initialized.
type RepositoryNotInitializedError struct{}

func (e *RepositoryNotInitializedError) Error() string {
	return "repository not initialized"
}

// StoreWAL stores a WAL file inside the repository.
func (s *impl) StoreWAL(ctx context.Context, name string, content []byte) error {
	if !s.IsReady() {
		return errors.New("tier1 is not yet ready")
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

// GetLatestWALFileName returns the latest WAL file that have been archived.
func (s *impl) GetLatestWALFileName(ctx context.Context) (string, error) {
	if !s.IsReady() {
		return "", errors.New("tier1 is not yet ready")
	}

	//nolint:godox
	// TODO: this doesn't scale well, we need to find a better solution.
	manifests, err := s.repository.FindManifests(ctx, s.getSnapshotRepositoryLabels())
	if err != nil {
		return "", fmt.Errorf("while finding manifests: %w", err)
	}

	return getLatestWALFileNameFromManifests(manifests, s.segmentSize)
}

func getLatestWALFileNameFromManifests(manifests []*manifest.EntryMetadata, walSegmentSize uint64) (string, error) {
	if len(manifests) == 0 {
		return "", fmt.Errorf("no manifests passed")
	}

	type wal struct {
		name string
		lsn  types.LSN
	}

	firstWALName := path.Base(manifests[0].Labels["path"])
	firstWALLsn, err := types.LSNStartFromWALName(firstWALName, walSegmentSize)
	if err != nil {
		return "", err
	}
	latestWAL := wal{
		name: firstWALName,
		lsn:  firstWALLsn,
	}

	for _, m := range manifests {
		lsn, err := types.LSNStartFromWALName(path.Base(m.Labels["path"]), walSegmentSize)
		if err != nil {
			return "", err
		}

		if lsn > latestWAL.lsn {
			latestWAL = wal{
				name: path.Base(m.Labels["path"]),
				lsn:  lsn,
			}
		}
	}

	return latestWAL.name, nil
}

func (s *impl) GetWAL(ctx context.Context, walName string) (*WalEntry, error) {
	if !s.IsReady() {
		return nil, errors.New("tier1 is not yet ready")
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
		return nil, &WALNotFoundError{walName: walName}
	}

	mf, err := snapshot.LoadSnapshot(ctx, s.repository, walManifest.ID)
	if err != nil {
		return nil, fmt.Errorf("while loading snapshot: %w", err)
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

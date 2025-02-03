package kopia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/cloudnative-pg/machinery/pkg/types"
	"github.com/kopia/kopia/fs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"

	klioTypes "github.com/EnterpriseDB/klio/pkg/klioclient/types"
)

// ErrNoManifestsPassed is raised when the Klio repository is empty.
var ErrNoManifestsPassed = fmt.Errorf("no manifests passed")

// StoreWAL stores a WAL file inside the repository.
func (s *Connection) StoreWAL(ctx context.Context, name string, content []byte) error {
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
		Host:     s.hostname,
		UserName: s.username,
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
func (s *Connection) GetLatestWALFileName(ctx context.Context, segmentSize uint64) (string, error) {
	//nolint:godox
	// TODO: this doesn't scale well, we need to find a better solution.
	manifests, err := s.repository.FindManifests(ctx, s.getSnapshotRepositoryLabels())
	if err != nil {
		return "", fmt.Errorf("while finding manifests: %w", err)
	}

	return getLatestWALFileNameFromManifests(manifests, segmentSize)
}

func getLatestWALFileNameFromManifests(manifests []*manifest.EntryMetadata, walSegmentSize uint64) (string, error) {
	if len(manifests) == 0 {
		return "", ErrNoManifestsPassed
	}

	type wal struct {
		name string
		lsn  types.LSN
	}

	firstWALName := path.Base(manifests[0].Labels["path"])
	firstWALLsn, err := types.LSNStartFromWALName(firstWALName, walSegmentSize)
	if err != nil {
		return "", fmt.Errorf("while getting latest WAL file from manifests: %w", err)
	}
	latestWAL := wal{
		name: firstWALName,
		lsn:  firstWALLsn,
	}

	for _, manifest := range manifests {
		lsn, err := types.LSNStartFromWALName(path.Base(manifest.Labels["path"]), walSegmentSize)
		if err != nil {
			return "", fmt.Errorf("while getting latest WAL file from manifests: %w", err)
		}

		if lsn > latestWAL.lsn {
			latestWAL = wal{
				name: path.Base(manifest.Labels["path"]),
				lsn:  lsn,
			}
		}
	}

	return latestWAL.name, nil
}

// ErrWALFileNotFound is raised when no WAL has been found.
var ErrWALFileNotFound = errors.New("wal file not found")

// ErrDuplicateManifestEntries is raised when there are two Kopia manifests
// for the same source entry.
var ErrDuplicateManifestEntries = errors.New("duplicate WAL manifest found, repository is corrupted")

// GetWAL downloads a WAL file from the Klio server.
func (s *Connection) GetWAL(ctx context.Context, walName string) (*klioTypes.WalEntry, error) {
	kopiaPath := path.Join("/wal", walName)

	sourceInfo := snapshot.SourceInfo{
		Host:     s.hostname,
		UserName: s.username,
		Path:     kopiaPath,
	}

	manifests, err := snapshot.ListSnapshotManifests(ctx, s.repository, &sourceInfo, s.getSnapshotRepositoryLabels())
	if err != nil {
		return nil, fmt.Errorf("while finding manifests: %w", err)
	}

	if len(manifests) == 0 {
		return nil, ErrWALFileNotFound
	}
	if len(manifests) > 1 {
		return nil, ErrDuplicateManifestEntries
	}

	walManifestID := manifests[0]

	mf, err := snapshot.LoadSnapshot(ctx, s.repository, walManifestID)
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

	return klioTypes.NewWalEntry(walName, readerData), nil
}

func (s *Connection) getSnapshotRepositoryLabels() map[string]string {
	return map[string]string{
		"type":     "snapshot",
		"hostname": s.hostname,
		"username": s.username,
		"content":  "wal",
	}
}

//nolint:ireturn
func getWALFileEntry(walName string, content []byte) fs.File {
	return klioTypes.NewWalEntry(walName, content)
}

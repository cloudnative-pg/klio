package tier1

import (
	"context"
	"fmt"
	"path"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"
)

// StoreWAL stores a WAL file inside the repository
func (s *impl) StoreWAL(ctx context.Context, name string, content []byte) error {
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
	manifest.Tags = map[string]string{}

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

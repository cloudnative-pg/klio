package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"

	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/snapshotfs"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
)

// manifestIDAnnotationName is the name of the annotation where
// the Kopia manifest ID is stored.
const manifestIDAnnotationName = "klio.io/kopiaManifestID"

// backupNameTagName is the name of the tag containing the backup
// name
const backupNameTagName = "klio.io/tag"

type backupImplementation struct {
	hostname   string
	username   string
	logger     *slog.Logger
	repository repo.Repository
	progress   common.UploadProgress

	pgDataManifest *snapshot.Manifest
	tablespaces    []common.TablespaceLayout
}

// CreateBackupExecutor creates a new backup executor.
func (s *Connection) CreateBackupExecutor(
	_ context.Context,
	options common.BackupOptions,
) (*common.BackupExecutor, error) {
	impl := &backupImplementation{
		hostname:   s.hostname,
		username:   s.username,
		logger:     s.logger,
		repository: s.repository,
		progress:   options.Progress,
	}

	return common.NewBackupExecutorForImpl(impl, options), nil
}

// UploadPgData implements common.BackupExecutorImplementation.
func (impl *backupImplementation) UploadPgData(ctx context.Context, pgData string) error {
	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("backing up pgdata %s for cluster %s", pgData, impl.hostname),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}

	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			impl.logger.Error("while closing repository write session to archive WALs", "err", err)
		}
	}()

	impl.pgDataManifest, err = impl.uploadPath(ctx, pgData, writer)
	if err != nil {
		return err
	}

	impl.pgDataManifest.Tags = map[string]string{
		"content": "pgdata",
	}

	if err = writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

// UploadTablespace implements common.BackupExecutorImplementation.
func (impl *backupImplementation) UploadTablespace(ctx context.Context, tbl common.TablespaceLayout) error {
	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("backing up tablespace %s for cluster %s", tbl.Path, impl.hostname),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}

	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			impl.logger.Error("while closing repository write session to archive WALs", "err", err)
		}
	}()

	manifest, err := impl.uploadPath(ctx, tbl.Path, writer)
	if err != nil {
		return err
	}

	manifest.Tags = map[string]string{
		"content":         "tablespace",
		"tablespace_name": path.Base(tbl.Name),
		"oid":             fmt.Sprintf("%v", tbl.Oid),
	}

	tablespaceManifestID, err := snapshot.SaveSnapshot(ctx, writer, manifest)
	if err != nil {
		return fmt.Errorf("while saving manifest ID to repository: %w", err)
	}

	if tbl.Annotations == nil {
		tbl.Annotations = make(map[string]string)
	}
	tbl.Annotations[manifestIDAnnotationName] = string(tablespaceManifestID)

	impl.tablespaces = append(impl.tablespaces, tbl)

	if err = writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

// FinishBackup implements common.BackupExecutorImplementation.
func (impl *backupImplementation) FinishBackup(ctx context.Context, data common.BackupMetadata) error {
	// This enables Kopia debugging
	// ctx = logging.WithLogger(ctx, logging.ToWriter(os.Stdout))
	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("archiving backup metadata %s", data.Name),
	})
	if err != nil {
		return fmt.Errorf("while creating repository writer session: %w", err)
	}

	defer func() {
		err := writer.Close(ctx)
		if err != nil {
			impl.logger.Error("while closing repository write session to archive WALs", "err", err)
		}
	}()

	data.Tablespaces = impl.tablespaces
	metadataContent, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("while marshalling metadata: %w", err)
	}
	impl.pgDataManifest.Description = string(metadataContent)

	if impl.pgDataManifest.Tags == nil {
		impl.pgDataManifest.Tags = make(map[string]string)
	}
	impl.pgDataManifest.Tags[backupNameTagName] = data.Name

	pgDataManifestID, err := snapshot.SaveSnapshot(ctx, writer, impl.pgDataManifest)
	if err != nil {
		return fmt.Errorf("while saving manifest ID to repository: %w", err)
	}
	impl.logger.Debug("Saved PGData Snapshot", "manifestID", pgDataManifestID)

	err = writer.Flush(ctx)
	if err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

func (impl *backupImplementation) uploadPath(
	ctx context.Context,
	filePath string,
	writer repo.RepositoryWriter,
) (*snapshot.Manifest, error) {
	// Kopia backup mode
	// ctx = logging.WithLogger(ctx, logging.ToWriter(os.Stdout))
	sourcePath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("error while looking for absolute path of %s: %w", filePath, err)
	}

	sourceInfo := snapshot.SourceInfo{
		Host:     impl.hostname,
		UserName: impl.username,
		Path:     filepath.Clean(sourcePath),
	}

	policyTree, err := policy.TreeForSource(ctx, impl.repository, sourceInfo)
	if err != nil {
		return nil, fmt.Errorf("while getting policy tree: %w", err)
	}

	entry, err := localfs.NewEntry(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("error while looking for source entry of %s: %w", sourcePath, err)
	}

	manifestsIDs, err := snapshot.ListSnapshotManifests(ctx, impl.repository, &sourceInfo, nil)
	if err != nil {
		return nil, fmt.Errorf("while finding manifests: %w", err)
	}

	manifests, err := snapshot.LoadSnapshots(ctx, impl.repository, manifestsIDs)
	if err != nil {
		return nil, fmt.Errorf("while loading manifests: %w", err)
	}

	uploader := snapshotfs.NewUploader(writer)
	uploader.Progress = &kopiaUploadProgress{
		startPath: sourcePath,
		p:         impl.progress,
		log:       impl.logger,
	}

	manifest, err := uploader.Upload(ctx, entry, policyTree, sourceInfo, manifests...)
	if err != nil {
		return nil, fmt.Errorf("while uploading to archive: %w", err)
	}

	return manifest, nil
}

package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/upload"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
	"github.com/EnterpriseDB/klio/internal/client/klioclient/notifier"
)

// pgDataManifestIDAnnotationName is the name of the annotation where
// the Kopia manifest ID is stored.
const pgDataManifestIDAnnotationName = "klio.io/kopiaManifestID"

// tablespaceManifestIDAnnotationName is the name of the annotation where
// the Kopia manifest ID of a tablespace is stored.
const tablespaceManifestIDAnnotationName = "klio.io/kopiaManifestID"

// controlDataManifestIDAnnotationName is the name of the annotation where
// the Kopia manifest ID of the pg control file is stored.
const controlDataManifestIDAnnotationName = "klio.io/controlDataKopiaManifestID"

// backupNameTagName is the name of the tag containing the backup
// name.
const backupNameTagName = "klio.io/tag"

type backupUploader struct {
	hostname   string
	username   string
	logger     *slog.Logger
	repository repo.Repository
	progress   notifier.Upload

	pgDataManifest        *snapshot.Manifest
	tablespaces           []common.TablespaceLayout
	controlDataManifestID manifest.ID
}

// NewUploader creates a new backup executor.
func (s *Connection) NewUploader(_ context.Context, logger notifier.Upload) common.BackupUploader { //nolint:ireturn
	return &backupUploader{
		hostname:   s.hostname,
		username:   s.username,
		logger:     s.logger,
		repository: s.repository,
		progress:   logger,
	}
}

// UploadPgData implements common.BackupUploader.
func (impl *backupUploader) UploadPgData(ctx context.Context, pgData string) error {
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

	// Step 1: add a .kopiaignore file to avoid backing up
	// the irrelevant part of the PGDATA directory.
	//
	// IMPORTANT: yes this is ugly, but I didn't found a way
	// to achieve a similar behaviour just using the Kopia
	// API
	kopiaIgnore := path.Join(pgData, kopiaIgnoreFileName)
	if err := os.WriteFile(kopiaIgnore, []byte(kopiaIgnoreContent), 0o600); err != nil {
		return fmt.Errorf("while creating .kopiaignore file: %w", err)
	}

	// Step 2: upload PGDATA to the target Kopia server
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

	// Step 3: remove .kopiaignore file from PGDATA
	if err := os.Remove(kopiaIgnore); err != nil {
		impl.logger.Warn("cannot remove .kopiaignore file", "file", kopiaIgnore)
	}

	return nil
}

// UploadTablespace implements common.BackupUploader.
func (impl *backupUploader) UploadTablespace(ctx context.Context, tbl common.TablespaceLayout) error {
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
		"oid":             tbl.Oid,
	}

	tablespaceManifestID, err := snapshot.SaveSnapshot(ctx, writer, manifest)
	if err != nil {
		return fmt.Errorf("while saving manifest ID to repository: %w", err)
	}

	if tbl.Annotations == nil {
		tbl.Annotations = make(map[string]string)
	}
	tbl.Annotations[tablespaceManifestIDAnnotationName] = string(tablespaceManifestID)

	impl.tablespaces = append(impl.tablespaces, tbl)

	if err = writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

// UploadControlFile implements common.BackupUploader.
func (impl *backupUploader) UploadControlFile(
	ctx context.Context,
	controlDataFileName string,
) error { // This enables Kopia debugging
	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: "backing up control file for cluster " + impl.hostname,
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

	manifest, err := impl.uploadPath(ctx, controlDataFileName, writer)
	if err != nil {
		return err
	}

	manifest.Tags = map[string]string{
		"content": "controldata",
	}

	controlDataManifestID, err := snapshot.SaveSnapshot(ctx, writer, manifest)
	if err != nil {
		return fmt.Errorf("while saving manifest ID to repository: %w", err)
	}

	if err = writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	impl.controlDataManifestID = controlDataManifestID

	return nil
}

// UploadBackupMetadata implements common.BackupUploader.
func (impl *backupUploader) UploadBackupMetadata(ctx context.Context, data common.BackupMetadata) error {
	// This enables Kopia debugging
	// ctx = logging.WithLogger(ctx, logging.ToWriter(os.Stdout))
	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: "archiving backup metadata " + data.Name,
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

	if data.Annotations == nil {
		data.Annotations = make(map[string]string)
	}
	data.Annotations[controlDataManifestIDAnnotationName] = string(impl.controlDataManifestID)

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

func (impl *backupUploader) uploadPath(
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

	uploader := upload.NewUploader(writer)
	uploader.Progress = &kopiaUploadProgress{
		startPath: sourcePath,
		notifier:  impl.progress,
		log:       impl.logger,
	}

	manifest, err := uploader.Upload(ctx, entry, policyTree, sourceInfo, manifests...)
	if err != nil {
		return nil, fmt.Errorf("while uploading to archive: %w", err)
	}

	return manifest, nil
}

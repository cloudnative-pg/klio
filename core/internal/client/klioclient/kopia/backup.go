package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/kopia/kopia/fs/localfs"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/policy"
	"github.com/kopia/kopia/snapshot/upload"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/notifier"
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

// BackupUploader is the implementation based on a Kopia client
// of the common.BackupUploader interface.
type BackupUploader struct {
	hostname   string
	username   string
	repository repo.Repository

	pgDataManifest        *snapshot.Manifest
	tablespaces           []common.TablespaceLayout
	controlDataManifestID manifest.ID

	sources []string
}

// Target is used to point a Kopia transaction to the set of snapshots
// having the specified Hostname and Username.
type Target struct {
	// Hostname is the hostname of the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Hostname string

	// Username is the name of the user that took the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Username string

	// Sources are the list of Kopia sources that have been uploaded
	Sources []string
}

// NewUploaderFor creates a new backup executor.
func (s *Connection) NewUploaderFor(t Target) *BackupUploader {
	return &BackupUploader{
		hostname:   t.Hostname,
		username:   t.Username,
		repository: s.repository,
	}
}

// UploadPgData implements common.BackupUploader.
func (impl *BackupUploader) UploadPgData(ctx context.Context, pgData string) error {
	contextLogger := log.FromContext(ctx)

	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("backing up pgdata %s for cluster %s", pgData, impl.hostname),
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
		contextLogger.Warning("cannot remove .kopiaignore file", "file", kopiaIgnore)
	}

	return nil
}

// UploadTablespace implements common.BackupUploader.
func (impl *BackupUploader) UploadTablespace(ctx context.Context, tbl common.TablespaceLayout) error {
	contextLogger := log.FromContext(ctx)

	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: fmt.Sprintf("backing up tablespace %s for cluster %s", tbl.Path, impl.hostname),
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
		return fmt.Errorf(
			"while saving manifest ID to repository (tablespace %q, cluster %q, user %q): %w",
			tbl.Name,
			impl.hostname,
			impl.username,
			err)
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
func (impl *BackupUploader) UploadControlFile(
	ctx context.Context,
	controlDataFileName string,
) error { // This enables Kopia debugging
	contextLogger := log.FromContext(ctx)

	ctx, writer, err := impl.repository.NewWriter(ctx, repo.WriteSessionOptions{
		Purpose: "backing up control file for cluster " + impl.hostname,
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

	manifest, err := impl.uploadPath(ctx, controlDataFileName, writer)
	if err != nil {
		return err
	}

	manifest.Tags = map[string]string{
		"content": "controldata",
	}

	controlDataManifestID, err := snapshot.SaveSnapshot(ctx, writer, manifest)
	if err != nil {
		return fmt.Errorf(
			"while saving manifest ID to repository (pg_controldata of cluster %q, user %q): %w",
			impl.hostname,
			impl.username,
			err,
		)
	}

	if err = writer.Flush(ctx); err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	impl.controlDataManifestID = controlDataManifestID

	return nil
}

// UploadBackupMetadata implements common.BackupUploader.
func (impl *BackupUploader) UploadBackupMetadata(ctx context.Context, data *common.BackupMetadata) error {
	contextLogger := log.FromContext(ctx)

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
			contextLogger.Error(err, "while closing repository write session to archive WALs")
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
		return fmt.Errorf(
			"while saving manifest ID to repository (pgdata of cluster %q user %q): %w",
			impl.hostname,
			impl.username,
			err,
		)
	}
	contextLogger.Debug("Saved PGData Snapshot", "manifestID", pgDataManifestID)

	err = writer.Flush(ctx)
	if err != nil {
		return fmt.Errorf("while flushing repo: %w", err)
	}

	return nil
}

// Sources implements the backup uploader interface.
func (impl *BackupUploader) Sources() []string {
	return impl.sources
}

func (impl *BackupUploader) uploadPath(
	ctx context.Context,
	filePath string,
	writer repo.RepositoryWriter,
) (*snapshot.Manifest, error) {
	contextLogger := log.FromContext(ctx)

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
		notifier:  notifier.NewUploadLogNotifier(contextLogger),
		log:       contextLogger,
	}

	manifest, err := uploader.Upload(ctx, entry, policyTree, sourceInfo, manifests...)
	if err != nil {
		return nil, fmt.Errorf("while uploading to archive: %w", err)
	}

	impl.sources = append(impl.sources, sourceInfo.String())

	return manifest, nil
}

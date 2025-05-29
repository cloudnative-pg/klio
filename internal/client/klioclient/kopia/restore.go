package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/restore"
	"github.com/kopia/kopia/snapshot/snapshotfs"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
)

type restoreImplementation struct {
	hostname   string
	username   string
	logger     *slog.Logger
	repository repo.Repository
	progress   common.DownloadProgress
}

// CreateRestoreExecutor creates a restore executor using the kopia
// client.
func (s *Connection) CreateRestoreExecutor(
	_ context.Context,
	opts common.RestoreOptions,
) (*common.RestoreExecutor, error) {
	impl := &restoreImplementation{
		hostname:   s.hostname,
		username:   s.username,
		logger:     s.logger,
		repository: s.repository,
		progress:   opts.Progress,
	}

	return common.NewRestoreExecutorForImpl(impl, opts), nil
}

// GetMetadata implements the RestoreExecutor interface.
func (s *restoreImplementation) GetMetadata(ctx context.Context, name string) (*common.BackupMetadata, error) {
	// Look for the kopia manifest with that name
	entries, err := s.repository.FindManifests(ctx, map[string]string{
		backupNameTagName: name,
	})
	if err != nil {
		return nil, fmt.Errorf("while looking for backup entry: %w", err)
	}
	if len(entries) > 1 {
		return nil, newMultipleBackupsFoundError(name, len(entries))
	}
	if len(entries) == 0 {
		return nil, newNoBackupFoundError(name)
	}

	manifest, err := snapshot.LoadSnapshot(ctx, s.repository, entries[0].ID)
	if err != nil {
		return nil, fmt.Errorf("while loading snapshot from manifest ID %q: %w", entries[0].ID, err)
	}

	var metadata common.BackupMetadata
	if err := json.Unmarshal([]byte(manifest.Description), &metadata); err != nil {
		return nil, fmt.Errorf("while unmarshalling backup description for %q: %w", name, err)
	}

	if metadata.Annotations == nil {
		metadata.Annotations = make(map[string]string)
	}
	metadata.Annotations[pgDataManifestIDAnnotationName] = string(manifest.ID)

	return &metadata, nil
}

// RestoreTablespace implements the RestoreExecutor interface.
func (s *restoreImplementation) RestoreTablespace(
	ctx context.Context,
	tbl common.TablespaceLayout,
	destinationDirectory string,
) error {
	restoreOutput, err := getFSOutput(ctx, destinationDirectory)
	if err != nil {
		return err
	}

	restoreOptions := s.getKopiaRestoreOptions(destinationDirectory)

	tablespaceManifestID := tbl.Annotations[controlDataManifestIDAnnotationName]

	root, err := snapshot.LoadSnapshot(ctx, s.repository, manifest.ID(tablespaceManifestID))
	if err != nil {
		return fmt.Errorf(
			"while loading snapshot %q for tablespace %q: %w",
			tablespaceManifestID,
			tbl.Name,
			err,
		)
	}

	entry, err := snapshotfs.SnapshotRoot(s.repository, root)
	if err != nil {
		return fmt.Errorf(
			"while recoverying snapshot root for manifest (tablespace %q) %q: %w",
			tbl.Name,
			tablespaceManifestID,
			err,
		)
	}

	if _, err := restore.Entry(ctx, s.repository, restoreOutput, entry, restoreOptions); err != nil {
		return fmt.Errorf("while restoring entry: %w", err)
	}

	return nil
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *restoreImplementation) RestorePgData(
	ctx context.Context,
	metadata *common.BackupMetadata,
	destinationDirectory string,
) error {
	restoreOutput, err := getFSOutput(ctx, destinationDirectory)
	if err != nil {
		return err
	}

	restoreOptions := s.getKopiaRestoreOptions(destinationDirectory)
	manifestID := metadata.Annotations[pgDataManifestIDAnnotationName]

	root, err := snapshot.LoadSnapshot(ctx, s.repository, manifest.ID(manifestID))
	if err != nil {
		return fmt.Errorf(
			"while loading snapshot %q for pgdata: %w",
			manifestID,
			err,
		)
	}

	entry, err := snapshotfs.SnapshotRoot(s.repository, root)
	if err != nil {
		return fmt.Errorf(
			"while recoverying snapshot root for manifest (pgdata) %q: %w",
			manifestID,
			err,
		)
	}

	if _, err := restore.Entry(ctx, s.repository, restoreOutput, entry, restoreOptions); err != nil {
		return fmt.Errorf("while restoring entry: %w", err)
	}

	return nil
}

func (s *restoreImplementation) RestoreControlData(
	ctx context.Context,
	metadata *common.BackupMetadata,
	destinationPath string,
) error {
	restoreOutput, err := getFSOutput(ctx, destinationPath)
	if err != nil {
		return err
	}

	restoreOptions := s.getKopiaRestoreOptions(destinationPath)
	controlDataManifestID := metadata.Annotations[controlDataManifestIDAnnotationName]

	root, err := snapshot.LoadSnapshot(ctx, s.repository, manifest.ID(controlDataManifestID))
	if err != nil {
		return fmt.Errorf(
			"while loading snapshot %q for controldata: %w",
			controlDataManifestID,
			err,
		)
	}

	entry, err := snapshotfs.SnapshotRoot(s.repository, root)
	if err != nil {
		return fmt.Errorf(
			"while recoverying snapshot root for manifest (controldata) %q: %w",
			controlDataManifestID,
			err,
		)
	}

	_, err = restore.Entry(ctx, s.repository, restoreOutput, entry, restoreOptions)
	if err != nil {
		return fmt.Errorf("while restoring entry: %w", err)
	}

	return nil
}

// getFSOutput creates the file system output representation for
// the Kopia API.
func getFSOutput(ctx context.Context, directory string) (*restore.FilesystemOutput, error) {
	result := restore.FilesystemOutput{
		TargetPath:             directory,
		OverwriteDirectories:   true,
		OverwriteFiles:         true,
		OverwriteSymlinks:      true,
		IgnorePermissionErrors: false,
		WriteFilesAtomically:   false,
		SkipOwners:             false,
		SkipPermissions:        false,
		SkipTimes:              false,
		WriteSparseFiles:       false,
	}

	if err := result.Init(ctx); err != nil {
		return nil, fmt.Errorf("while initializing restore options: %w", err)
	}

	return &result, nil
}

// getKopiaProgressCallback converts a common.DownloadProgressCallback
// to a callback suitable for the Kopia API.
func (s *restoreImplementation) getKopiaProgressCallback(destinationDirectory string) restore.ProgressCallback {
	if s.progress == nil {
		return nil
	}

	return func(_ context.Context, stats restore.Stats) {
		s.progress.NotifyStatus(destinationDirectory, common.DownloadStats{
			RestoredTotalFileSize: stats.RestoredTotalFileSize,
			EnqueuedTotalFileSize: stats.EnqueuedTotalFileSize,
			SkippedTotalFileSize:  stats.SkippedTotalFileSize,
			RestoredFileCount:     stats.RestoredFileCount,
			RestoredDirCount:      stats.RestoredDirCount,
			RestoredSymlinkCount:  stats.RestoredSymlinkCount,
			EnqueuedFileCount:     stats.EnqueuedFileCount,
			EnqueuedDirCount:      stats.EnqueuedDirCount,
			EnqueuedSymlinkCount:  stats.EnqueuedSymlinkCount,
			SkippedCount:          stats.SkippedCount,
			IgnoredErrorCount:     stats.IgnoredErrorCount,
		})
	}
}

// getKopiaRestoreOptions gets the common kopia restore options
// to be used by PgData and by tablespaces.
func (s *restoreImplementation) getKopiaRestoreOptions(destinationDirectory string) restore.Options {
	return restore.Options{
		Parallel:               0,
		Incremental:            true,
		IgnoreErrors:           false,
		ProgressCallback:       s.getKopiaProgressCallback(destinationDirectory),
		RestoreDirEntryAtDepth: math.MaxInt32,
	}
}

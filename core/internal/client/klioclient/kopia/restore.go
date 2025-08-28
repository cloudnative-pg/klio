package kopia

import (
	"context"
	"fmt"
	"math"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/manifest"
	"github.com/kopia/kopia/snapshot"
	"github.com/kopia/kopia/snapshot/restore"
	"github.com/kopia/kopia/snapshot/snapshotfs"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/notifier"
)

// RestoreImplementation is an implementation of common.BackupRestorer.
type RestoreImplementation struct {
	hostname   string
	username   string
	repository repo.Repository
	notifier   notifier.Download
}

// GetDownloadNotifier returns the notifier used by the RestoreImplementation.
//
//nolint:nolintlint,ireturn
func (s *RestoreImplementation) GetDownloadNotifier() notifier.Download {
	return s.notifier
}

// CreateRestorer creates a restore executor using the kopia
// client.
func (s *Connection) CreateRestorer(notifier notifier.Download) *RestoreImplementation {
	return &RestoreImplementation{
		hostname:   s.hostname,
		username:   s.username,
		repository: s.repository,
		notifier:   notifier,
	}
}

// RestoreTablespace implements the RestoreExecutor interface.
func (s *RestoreImplementation) RestoreTablespace(
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
func (s *RestoreImplementation) RestorePgData(
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

// RestoreControlData restores the control data from the backup.
func (s *RestoreImplementation) RestoreControlData(
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
func (s *RestoreImplementation) getKopiaProgressCallback(destinationDirectory string) restore.ProgressCallback {
	return func(_ context.Context, stats restore.Stats) {
		s.notifier.NotifyStatus(destinationDirectory, notifier.DownloadStats{
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
func (s *RestoreImplementation) getKopiaRestoreOptions(destinationDirectory string) restore.Options {
	return restore.Options{
		Parallel:               0,
		Incremental:            true,
		IgnoreErrors:           false,
		ProgressCallback:       s.getKopiaProgressCallback(destinationDirectory),
		RestoreDirEntryAtDepth: math.MaxInt32,
	}
}

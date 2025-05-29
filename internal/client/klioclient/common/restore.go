package common

import (
	"context"
	"fmt"
	"os"
	"path"
)

// backupLabelFileName is the file name where the backup label should be stored.
const backupLabelFileName = "backup_label"

// RestoreExecutorImplementation is used by a restore executor to download
// data from the backup store to the local file system.
type RestoreExecutorImplementation interface {
	// GetMetadata gets the backup metadata from the backup store given
	// the backup name.
	GetMetadata(ctx context.Context, name string) (*BackupMetadata, error)

	// RestoreTablespace restores the passed tablespace in the specified
	// folder.
	RestoreTablespace(ctx context.Context, tbl TablespaceLayout, destinationDirectory string) error

	// RestorePgData restores the passed pgdata in the specified
	// directory.
	RestorePgData(ctx context.Context, metadata *BackupMetadata, destinationDirectory string) error

	// RestoreControlData restores the pg control data file into
	// the passed file name.
	RestoreControlData(ctx context.Context, metadata *BackupMetadata, destinationPath string) error
}

// RestoreExecutor guides the execution of a restore process, delegating
// the download of data to the underlying implementation.
type RestoreExecutor struct {
	impl    RestoreExecutorImplementation
	options RestoreOptions
}

// RestoreOptions are the options that should be used by the restore
// process.
type RestoreOptions struct {
	// Name is the name of the backup that should be restored.
	Name string

	// PgDataDirectory is the target PGDATA where the files will be stored.
	PgDataDirectory string

	// TablespacesDirectory allow the user to customize the location
	// that will be used to download the tablespaces.
	TablespacesDirectory map[string]string

	// Progress is the set of callbacks to be used to report the restore
	// status. If null, no callback will be invoked.
	Progress DownloadProgress
}

// NewRestoreExecutorForImpl creates a new restore executor given
// a certain implementation.
func NewRestoreExecutorForImpl(impl RestoreExecutorImplementation, opts RestoreOptions) *RestoreExecutor {
	return &RestoreExecutor{
		impl:    impl,
		options: opts,
	}
}

// Restore handles the restore process.
func (r *RestoreExecutor) Restore(ctx context.Context, destinationPath string) error {
	meta, err := r.impl.GetMetadata(ctx, r.options.Name)
	if err != nil {
		return fmt.Errorf("while getting metadata for backup %s: %w", r.options.Name, err)
	}

	// Restore the tablespaces
	for _, tbl := range meta.Tablespaces {
		if err := r.restoreTablespace(ctx, tbl); err != nil {
			return fmt.Errorf("while restoring tablespace %s: %w", tbl.Name, err)
		}
	}

	// Restore PGDATA
	r.options.Progress.NotifyStart(destinationPath)
	if err := r.impl.RestorePgData(ctx, meta, destinationPath); err != nil {
		return fmt.Errorf("while restoring pgdata to %s: %w", destinationPath, err)
	}
	r.options.Progress.NotifyFinish(destinationPath)

	// Restore control data file
	controlDataFileName := path.Join(destinationPath, controlDataPath)
	r.options.Progress.NotifyStart(controlDataFileName)
	if err := r.impl.RestoreControlData(ctx, meta, controlDataFileName); err != nil {
		return fmt.Errorf("while restoring control data file to %s: %w", controlDataFileName, err)
	}
	r.options.Progress.NotifyFinish(controlDataFileName)

	// Restore backup label
	backupLabel := path.Join(destinationPath, backupLabelFileName)
	if err := os.WriteFile(backupLabel, []byte(meta.BackupLabel), 0o600); err != nil {
		return fmt.Errorf("while writing backup label %s file: %w", backupLabel, err)
	}

	return nil
}

func (r *RestoreExecutor) restoreTablespace(ctx context.Context, tbl TablespaceLayout) error {
	tablespaceDestinationPath := tbl.Path
	if v, ok := r.options.TablespacesDirectory[tbl.Name]; ok {
		tablespaceDestinationPath = v
	}

	r.options.Progress.NotifyStart(tablespaceDestinationPath)
	if err := r.impl.RestoreTablespace(ctx, tbl, tablespaceDestinationPath); err != nil {
		return fmt.Errorf("while restoring tablespace %s to %s: %w", tbl.Name, tablespaceDestinationPath, err)
	}
	defer r.options.Progress.NotifyFinish(tablespaceDestinationPath)

	return nil
}

package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// UploadTablespace implements common.BackupUploader.
func (s *Connection) UploadTablespace(
	ctx context.Context,
	backupName string,
	tbl klioclient.TablespaceLayout,
) error {
	tags := map[string]string{
		klioclient.BackupContentTagName:  "tablespace",
		klioclient.TablespaceNameTagName: path.Base(tbl.Name),
		klioclient.BackupNameTagName:     backupName,
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   tbl.Path,
		Tags:        tags,
		Description: fmt.Sprintf("tablespace %s (%v)", tbl.Name, tbl.Oid),
	})
	if err != nil {
		return fmt.Errorf("unable to snapshot directory: %w", err)
	}

	return nil
}

// UploadPgData implements common.BackupUploader.
func (s *Connection) UploadPgData(ctx context.Context, backupName, pgData string) error {
	contextLogger := log.FromContext(ctx)

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
	tags := map[string]string{
		klioclient.BackupContentTagName: "pgdata",
		klioclient.BackupNameTagName:    backupName,
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   pgData,
		Tags:        tags,
		Description: "pgdata",
	})
	if err != nil {
		return err
	}

	// Step 3: remove .kopiaignore file from PGDATA
	if err := os.Remove(kopiaIgnore); err != nil {
		contextLogger.Warning("cannot remove .kopiaignore file", "file", kopiaIgnore)
	}

	return nil
}

// UploadControlFile implements common.BackupUploader.
func (s *Connection) UploadControlFile(ctx context.Context, backupName, controlDataFileName string) error {
	tags := map[string]string{
		klioclient.BackupNameTagName:    backupName,
		klioclient.BackupContentTagName: "controldata",
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   controlDataFileName,
		Tags:        tags,
		Description: "control data file",
	})
	if err != nil {
		return fmt.Errorf("while snapshotting control data file: %w", err)
	}

	return nil
}

// UploadBackupMetadata implements common.BackupUploader.
func (s *Connection) UploadBackupMetadata(
	ctx context.Context,
	backupName string,
	data *klioclient.BackupMetadata,
) error {
	metadataContent, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("while marshalling metadata: %w", err)
	}

	tags := map[string]string{
		klioclient.BackupNameTagName:    backupName,
		klioclient.BackupContentTagName: "metadata",
	}

	fakeMetadataDirectory := strings.TrimSuffix(data.PgData, "/") + "_meta"

	opts := kopia.SnapshotFileContentOptions{
		Content:       metadataContent,
		FileName:      "metadata.json",
		DirectoryName: fakeMetadataDirectory,
		Description:   "metadata for " + backupName,
		Tags:          tags,
	}

	if err := s.kopia.SnapshotFileContent(ctx, opts); err != nil {
		return fmt.Errorf("error file snapshotting metadata: %w", err)
	}

	return nil
}

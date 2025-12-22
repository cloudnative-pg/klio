package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
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

	err := s.uploadPath(ctx, tbl.Path, tags, fmt.Sprintf("tablespace %s (%v)", tbl.Name, tbl.Oid))
	if err != nil {
		return err
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

	err := s.uploadPath(ctx, pgData, tags, "pgdata")
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

	return s.uploadPath(ctx, controlDataFileName, tags, "control data file")
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

	content := uploadFile{
		content:       metadataContent,
		fileName:      "metadata.json",
		directoryName: fakeMetadataDirectory,
	}

	return s.uploadFile(ctx, content, tags, "metadata for "+backupName)
}

func (s *Connection) uploadPath(
	ctx context.Context,
	filePath string,
	tags map[string]string,
	description string,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"create",
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		"--config-file=" + s.configFile,
	}

	for k, v := range tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	if description != "" {
		args = append(args, "--description="+description)
	}

	args = append(args, filePath)

	snapshotCreateCommand := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Stdout = os.Stdout
	snapshotCreateCommand.Stderr = os.Stderr

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := snapshotCreateCommand.Run(); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	return nil
}

type uploadFile struct {
	fileName      string
	directoryName string
	content       []byte
}

func (s *Connection) uploadFile(
	ctx context.Context,
	content uploadFile,
	tags map[string]string,
	description string,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"create",
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		"--config-file=" + s.configFile,
		"--stdin-file=" + content.fileName,
		content.directoryName,
	}

	for k, v := range tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	if description != "" {
		args = append(args, "--description="+description)
	}

	buffer := bytes.NewBuffer(content.content)

	snapshotCreateCommand := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Stdin = buffer
	snapshotCreateCommand.Stdout = os.Stdout
	snapshotCreateCommand.Stderr = os.Stderr

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := snapshotCreateCommand.Run(); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	return nil
}

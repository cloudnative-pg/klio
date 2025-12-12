package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// kopiaCommand is the name of the kopia executable to be used.
const kopiaCommand = "kopia"

// backupNameTagName is the name of the tag containing the backup
// name.
const backupNameTagName = "klio.io/tag"

// backupContentTagName is the name of the tag containing the
// snapshot content.
const backupContentTagName = "klio.io/content"

// tablespaceNameTagName is the name of the tag containing the
// name of the tablespace.
const tablespaceNameTagName = "klio.io/tablespaceName"

// BackupUploader is the implementation based on a Kopia client
// of the common.BackupUploader interface.
type BackupUploader struct {
	hostname string
	username string

	configFile  string
	kopiaBinary string

	tablespaces []common.TablespaceLayout

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
}

// String formats this target as the string that the Kopia CLI
// would expect.
func (t Target) String() string {
	return fmt.Sprintf("%s@%s", t.Username, t.Hostname)
}

// NewUploaderFor creates a new backup executor.
func (s *Connection) NewUploaderFor(t Target) *BackupUploader {
	return &BackupUploader{
		hostname:    t.Hostname,
		username:    t.Username,
		configFile:  s.configFile,
		kopiaBinary: s.kopiaBinary,
	}
}

// UploadTablespace implements common.BackupUploader.
func (impl *BackupUploader) UploadTablespace(
	ctx context.Context,
	backupName string,
	tbl common.TablespaceLayout,
) error {
	tags := map[string]string{
		backupContentTagName:  "tablespace",
		tablespaceNameTagName: path.Base(tbl.Name),
		backupNameTagName:     backupName,
	}

	err := impl.uploadPath(ctx, tbl.Path, tags, fmt.Sprintf("tablespace %s (%v)", tbl.Name, tbl.Oid))
	if err != nil {
		return err
	}

	impl.tablespaces = append(impl.tablespaces, tbl)

	return nil
}

// UploadPgData implements common.BackupUploader.
func (impl *BackupUploader) UploadPgData(ctx context.Context, backupName, pgData string) error {
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
		backupContentTagName: "pgdata",
		backupNameTagName:    backupName,
	}

	err := impl.uploadPath(ctx, pgData, tags, "pgdata")
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
func (impl *BackupUploader) UploadControlFile(ctx context.Context, backupName, controlDataFileName string) error {
	tags := map[string]string{
		backupNameTagName:    backupName,
		backupContentTagName: "controldata",
	}

	return impl.uploadPath(ctx, controlDataFileName, tags, "control data file")
}

// UploadBackupMetadata implements common.BackupUploader.
func (impl *BackupUploader) UploadBackupMetadata(
	ctx context.Context,
	backupName string,
	data *common.BackupMetadata,
) error {
	data.Tablespaces = impl.tablespaces
	metadataContent, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("while marshalling metadata: %w", err)
	}

	tags := map[string]string{
		backupNameTagName:    backupName,
		backupContentTagName: "metadata",
	}

	fakeMetadataDirectory := strings.TrimSuffix(data.PgData, "/") + "_meta"

	content := uploadFile{
		content:       metadataContent,
		fileName:      "metadata.json",
		directoryName: fakeMetadataDirectory,
	}

	return impl.uploadFile(ctx, content, tags, "metadata for "+backupName)
}

// Sources implements the backup uploader interface.
func (impl *BackupUploader) Sources() []string {
	return impl.sources
}

func (impl *BackupUploader) uploadPath(
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
		"--config-file=" + impl.configFile,
	}

	for k, v := range tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	if description != "" {
		args = append(args, "--description="+description)
	}

	args = append(args, filePath)

	snapshotCreateCommand := exec.CommandContext(ctx, impl.kopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Stdout = os.Stdout
	snapshotCreateCommand.Stderr = os.Stderr

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := snapshotCreateCommand.Run(); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	impl.sources = append(
		impl.sources,
		fmt.Sprintf("%v@%v:%v", impl.username, impl.hostname, filepath.Clean(filePath)))

	return nil
}

type uploadFile struct {
	fileName      string
	directoryName string
	content       []byte
}

func (impl *BackupUploader) uploadFile(
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
		"--config-file=" + impl.configFile,
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

	snapshotCreateCommand := exec.CommandContext(ctx, impl.kopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Stdin = buffer
	snapshotCreateCommand.Stdout = os.Stdout
	snapshotCreateCommand.Stderr = os.Stderr

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := snapshotCreateCommand.Run(); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	impl.sources = append(
		impl.sources,
		fmt.Sprintf("%v@%v:%v", impl.username, impl.hostname, filepath.Clean(content.directoryName)))

	return nil
}

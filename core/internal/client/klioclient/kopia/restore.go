package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// RestoreImplementation is an implementation of common.BackupRestorer.
type RestoreImplementation struct {
	kopiaBinary string
	hostname    string
	username    string
	configFile  string
}

// CreateRestorer creates a restore executor using the kopia
// client.
func (s *Connection) CreateRestorer(t Target) *RestoreImplementation {
	return &RestoreImplementation{
		kopiaBinary: s.kopiaBinary,
		hostname:    t.Hostname,
		username:    t.Username,
		configFile:  s.configFile,
	}
}

// RestoreTablespace implements the RestoreExecutor interface.
func (s *RestoreImplementation) RestoreTablespace(
	ctx context.Context,
	metadata *common.BackupMetadata,
	tbl common.TablespaceLayout,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		backupContentTagName:  "tablespace",
		tablespaceNameTagName: tbl.Name,
		backupNameTagName:     metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationDirectory)
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *RestoreImplementation) RestorePgData(
	ctx context.Context,
	metadata *common.BackupMetadata,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		backupContentTagName: "pgdata",
		backupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationDirectory)
}

// RestoreControlData restores the control data from the backup.
func (s *RestoreImplementation) RestoreControlData(
	ctx context.Context,
	metadata *common.BackupMetadata,
	destinationPath string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		backupContentTagName: "controldata",
		backupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationPath)
}

// GetMetadata implements the RestoreExecutor interface.
func (s *RestoreImplementation) getSnapshotID(
	ctx context.Context,
	tags map[string]string,
) (string, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--password=mtls",
		"--config-file=" + s.configFile,
	}

	for k, v := range tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	var stdout bytes.Buffer
	snapshotList := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotList.Stdout = &stdout
	snapshotList.Stderr = os.Stderr

	contextLogger.Info("Looking for Kopia snapshot", "args", snapshotList.Args, "tags", tags)
	if err := snapshotList.Run(); err != nil {
		return "", fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return "", fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	for _, entry := range entries {
		if s.hostname != "" && entry.Source.Host == s.hostname {
			return entry.ID, nil
		}
	}

	return "", newNoSnapshotFound(s.hostname, tags)
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *RestoreImplementation) restoreSnapshot(
	ctx context.Context,
	snapshotID string,
	destinationDirectory string,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"restore",
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		snapshotID,
		destinationDirectory,
	}

	contextLogger.Info("Restoring Kopia snapshot", "args", args)

	restoreCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		return fmt.Errorf("while restoring Kopia snapshot: %w", err)
	}

	return nil
}

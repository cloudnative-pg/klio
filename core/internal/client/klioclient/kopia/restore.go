package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
)

// RestoreTablespace implements the RestoreExecutor interface.
func (s *Connection) RestoreTablespace(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	tbl klioclient.TablespaceLayout,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName:  "tablespace",
		klioclient.TablespaceNameTagName: tbl.Name,
		klioclient.BackupNameTagName:     metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationDirectory)
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *Connection) RestorePgData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationDirectory string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName: "pgdata",
		klioclient.BackupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationDirectory)
}

// RestoreControlData restores the control data from the backup.
func (s *Connection) RestoreControlData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationPath string,
) error {
	source, err := s.getSnapshotID(ctx, map[string]string{
		klioclient.BackupContentTagName: "controldata",
		klioclient.BackupNameTagName:    metadata.Name,
	})
	if err != nil {
		return err
	}

	return s.restoreSnapshot(ctx, source, destinationPath)
}

// getSnapshotID implements the RestoreExecutor interface.
func (s *Connection) getSnapshotID(
	ctx context.Context,
	tags map[string]string,
) (string, error) {
	contextLogger := log.FromContext(ctx)

	args := make([]string, 0, 7+len(tags))
	args = append(args,
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--password=mtls",
		"--config-file="+s.configFile,
	)

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

	var entries []klioclient.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return "", fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	for _, entry := range entries {
		if s.GetHostname() != "" && entry.Source.Host == s.GetHostname() {
			return entry.ID, nil
		}
	}

	return "", newNoSnapshotFound(s.GetHostname(), tags)
}

// RestorePgData restores the passed pgdata in the specified
// directory.
func (s *Connection) restoreSnapshot(
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

package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/common"
)

// GetMetadata implements the RestoreExecutor interface.
func (s *RestoreImplementation) GetMetadata(ctx context.Context, name string) (*common.BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--password=mtls",
		"--config-file=" + s.configFile,
		"--tags=" + backupNameTagName + ":" + name,
		"--tags=klio.io/content:metadata",
	}

	var stdout bytes.Buffer
	snapshotList := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotList.Stdout = &stdout
	snapshotList.Stderr = os.Stderr

	contextLogger.Info("Looking for Kopia backup", "args", snapshotList.Args)
	if err := snapshotList.Run(); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	for _, entry := range entries {
		if entry.Source.Host == s.hostname {
			metadata, err := s.restoreMetadata(ctx, entry.ID)
			if err != nil {
				return nil, err
			}

			return metadata, nil
		}
	}

	return nil, newNoBackupFoundError(name)
}

// ListBackups list all the backups in the repository.
func (s *RestoreImplementation) ListBackups(ctx context.Context) (common.BackupList, error) {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--password=mtls",
		"--config-file=" + s.configFile,
		"--tags=klio.io/content:metadata",
	}

	var stdout bytes.Buffer
	snapshotList := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	snapshotList.Stdout = &stdout
	snapshotList.Stderr = os.Stderr

	contextLogger.Info("Looking for Kopia backup", "args", snapshotList.Args)
	if err := snapshotList.Run(); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	result := make([]common.BackupMetadata, 0, len(entries))
	for _, entry := range entries {
		if s.hostname != "" && entry.Source.Host != s.hostname {
			continue
		}

		metadata, err := s.restoreMetadata(ctx, entry.ID)
		if err != nil {
			contextLogger.Error(err, "Error while decoding backup metadata, skipping", "id", entry.ID)
		} else {
			result = append(result, *metadata)
		}
	}

	return result, nil
}

// restoreMetadata restores the metadata stored in a snapshot with the
// given ID.
func (s *RestoreImplementation) restoreMetadata(
	ctx context.Context,
	snapshotID string,
) (*common.BackupMetadata, error) {
	contextLogger := log.FromContext(ctx)

	dirName, err := os.MkdirTemp("", "kopia_snapshot_*")
	if err != nil {
		return nil, fmt.Errorf("allocating temporary directory to restore backup metadata: %w", err)
	}

	defer func() {
		err := os.RemoveAll(dirName)
		if err != nil {
			contextLogger.Error(err, "Error while cleaning up temporary directory, skipping", "dirName", dirName)
		}
	}()

	args := []string{
		"restore",
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		snapshotID,
		dirName,
	}

	contextLogger.Info("Restoring Kopia snapshot (metadata)", "args", args)

	restoreCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		return nil, fmt.Errorf("while restoring Kopia snapshot: %w", err)
	}

	metadataPath := filepath.Join(dirName, "metadata.json")
	f, err := os.Open(metadataPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening backup metadata: %w", err)
	}

	var result common.BackupMetadata
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		return nil, fmt.Errorf("cannot decode JSON backup metadata: %w", err)
	}

	return &result, nil
}

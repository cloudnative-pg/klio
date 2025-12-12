package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.uber.org/multierr"
)

// DeleteBackup removes the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, hostname string, name string) error {
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

	contextLogger.Info("Looking for Kopia backups", "args", snapshotList.Args)
	if err := snapshotList.Run(); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	var err error
	for _, entry := range entries {
		if entry.Source.Host == hostname {
			err = multierr.Append(err, s.internalDeleteSnapshot(ctx, entry.ID))
		}
	}

	return err
}

func (s *Connection) internalDeleteSnapshot(ctx context.Context, id string) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"delete",
		"--config-file=" + s.configFile,
		"--disable-file-logging",
		"--json-log-console",
		"--password=mtls",
		id,
	}

	contextLogger.Info("Deleting Kopia snapshot", "args", args)

	deleteSnapshotCmd := exec.CommandContext(ctx, s.kopiaBinary, args...) //nolint:gosec
	deleteSnapshotCmd.Stdout = os.Stdout
	deleteSnapshotCmd.Stderr = os.Stderr

	if err := deleteSnapshotCmd.Run(); err != nil {
		return fmt.Errorf("while deleting Kopia snapshot: %w", err)
	}

	return nil
}

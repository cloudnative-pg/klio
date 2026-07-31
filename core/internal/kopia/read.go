/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package kopia

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// LogFunc is a function type for logging messages with key-value pairs.
// It matches the signature of log.Logger.Info and log.Logger.Debug methods.
type LogFunc func(msg string, keysAndValues ...any)

// ListSnapshots returns a list of snapshots, optionally filtered by tags.
// The logFn parameter controls how the operation is logged - callers should pass
// contextLogger.Info for user-facing operations or contextLogger.Debug for
// internal/periodic operations.
func (s *Client) ListSnapshots(
	ctx context.Context,
	tags map[string]string,
	logFn LogFunc,
) ([]Manifest, error) {
	args := make([]string, 0, 6+len(tags))
	args = append(args,
		"snapshot",
		"list",
		"--disable-file-logging",
		"--all",
		"--json",
		"--config-file="+s.ConfigFile,
	)

	for k, v := range tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	var stdout bytes.Buffer
	snapshotList := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	snapshotList.Env = s.kopiaEnvironmentVariables()

	logFn("Looking for Kopia snapshots", "args", snapshotList.Args)

	if err := RunWithLogCapture(ctx, snapshotList, &stdout); err != nil {
		return nil, fmt.Errorf("while executing Kopia command: %w", err)
	}

	var entries []Manifest
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("while unmarshalling kopia command output %q: %w", stdout.String(), err)
	}

	return entries, nil
}

// RestoreSingleFile restores a single file from a snapshot and returns its
// contents. The logFn parameter controls how the operation is logged - callers
// should pass contextLogger.Info for user-facing operations or
// contextLogger.Debug for internal/periodic operations.
func (s *Client) RestoreSingleFile(
	ctx context.Context, snapshotID string, fileName string, logFn LogFunc,
) ([]byte, error) {
	contextLogger := log.FromContext(ctx)

	dirName, err := os.MkdirTemp("", "kopia_snapshot_*")
	if err != nil {
		return nil, fmt.Errorf("allocating temporary directory to restore single file: %w", err)
	}

	defer func() {
		err := os.RemoveAll(dirName)
		if err != nil {
			contextLogger.Error(err, "Error while cleaning up temporary directory, skipping", "dirName", dirName)
		}
	}()

	args := []string{
		"restore",
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		snapshotID,
		dirName,
	}

	logFn("Restoring Kopia snapshot (single file)", "args", args)

	restoreCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	restoreCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, restoreCmd, nil); err != nil {
		return nil, fmt.Errorf("while restoring Kopia snapshot: %w", err)
	}

	metadataPath := filepath.Join(dirName, fileName)
	f, err := os.Open(metadataPath) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening backup metadata: %w", err)
	}

	defer func() {
		closeErr := f.Close()
		if closeErr != nil {
			contextLogger.Error(closeErr, "Got error while closing file, skipping")
		}
	}()

	return io.ReadAll(f)
}

// RestoreSnapshot restores a snapshot to a destination directory.
func (s *Client) RestoreSnapshot(
	ctx context.Context,
	snapshotID string,
	destinationDirectory string,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"restore",
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		"--progress",
		"--progress-update-interval=60s",
		snapshotID,
		destinationDirectory,
	}

	contextLogger.Info("Restoring Kopia snapshot", "args", args)

	restoreCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	restoreCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, restoreCmd, nil); err != nil {
		return fmt.Errorf("while restoring Kopia snapshot: %w", err)
	}

	return nil
}

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
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"
)

// VerifyResult contains the parsed output from "kopia snapshot verify --json".
type VerifyResult struct {
	ErrorCount   int      `json:"errorCount"`
	ErrorStrings []string `json:"errorStrings,omitempty"`
}

// parseVerifyOutput parses the JSON output from "kopia snapshot verify --json".
// Returns a zero VerifyResult if the output cannot be parsed.
func parseVerifyOutput(stdout []byte) VerifyResult {
	var result VerifyResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return VerifyResult{}
	}

	return result
}

// DeleteSnapshot deletes a snapshot by its ID.
func (s *Client) DeleteSnapshot(ctx context.Context, id string) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"delete",
		"--delete",
		"--config-file=" + s.ConfigFile,
		"--disable-file-logging",
		id,
	}

	contextLogger.Info("Deleting Kopia snapshot", "args", args)

	deleteSnapshotCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	deleteSnapshotCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, deleteSnapshotCmd, nil); err != nil {
		return fmt.Errorf("while deleting Kopia snapshot: %w", err)
	}

	return nil
}

// SnapshotMigrateOpts contains options for migrating snapshots.
type SnapshotMigrateOpts struct {
	// SourceConfig is the path to the source repository configuration file.
	SourceConfig string

	// Sources is a list of source paths to migrate snapshots from.
	Sources []string

	// Tags is a list of tag names to filter which snapshots to migrate.
	Tags []string
}

// MigrateSnapshots migrates snapshots from a source configuration.
func (s *Client) MigrateSnapshots(
	ctx context.Context,
	opts SnapshotMigrateOpts,
) error {
	contextLogger := log.FromContext(ctx)

	args := make([]string, 0, 6+len(opts.Sources)+len(opts.Tags))
	args = append(args,
		"snapshot", "migrate",
		"--progress",
		"--progress-update-interval=60s",
		"--source-config="+opts.SourceConfig,
		"--config-file="+s.ConfigFile,
		"--disable-file-logging",
	)

	for _, source := range opts.Sources {
		args = append(args, "--sources="+source)
	}

	for _, tagName := range opts.Tags {
		args = append(args, "--tags=tag:"+tagName)
	}

	kopiaMigrate := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	kopiaMigrate.Env = s.kopiaEnvironmentVariables()

	contextLogger.Info("Starting Kopia migration", "args", kopiaMigrate.Args)

	if err := RunWithLogCapture(ctx, kopiaMigrate, nil); err != nil {
		return fmt.Errorf("while running the kopia migration: %w", err)
	}

	return nil
}

// SnapshotDirectoryOptions contains options for creating a directory snapshot.
type SnapshotDirectoryOptions struct {
	// Directory is the filesystem path to snapshot.
	Directory string

	// Tags contains user-defined key-value pairs to associate with the snapshot.
	Tags map[string]string

	// Description is a user-provided description of the snapshot.
	Description string

	// A pinned snapshot will not expire automatically.
	Pins []string
}

// SnapshotDirectory creates a snapshot of a directory.
func (s *Client) SnapshotDirectory(
	ctx context.Context,
	opts SnapshotDirectoryOptions,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"create",
		"--progress",
		"--progress-update-interval=60s",
		"--disable-file-logging",
		"--config-file=" + s.ConfigFile,
	}

	for k, v := range opts.Tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	for _, p := range opts.Pins {
		args = append(args, "--pin="+p)
	}

	if opts.Description != "" {
		args = append(args, "--description="+opts.Description)
	}

	args = append(args, opts.Directory)

	snapshotCreateCommand := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Env = s.kopiaEnvironmentVariables()

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := RunWithLogCapture(ctx, snapshotCreateCommand, nil); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	return nil
}

// SnapshotFileContentOptions contains options for creating a snapshot from file content.
type SnapshotFileContentOptions struct {
	// FileName is the name of the file within the snapshot.
	FileName string

	// DirectoryName is the directory path to use in the snapshot.
	DirectoryName string

	// Content is the in-memory file content to snapshot.
	Content []byte

	// Tags contains user-defined key-value pairs to associate with the snapshot.
	Tags map[string]string

	// Description is a user-provided description of the snapshot.
	Description string

	// A pinned snapshot will not expire automatically.
	Pins []string
}

// SnapshotFileContent creates a snapshot from in-memory file content.
func (s *Client) SnapshotFileContent(
	ctx context.Context,
	opts SnapshotFileContentOptions,
) error {
	contextLogger := log.FromContext(ctx)

	args := []string{
		"snapshot",
		"create",
		"--disable-file-logging",
		"--config-file=" + s.ConfigFile,
		"--stdin-file=" + opts.FileName,
		opts.DirectoryName,
	}

	for k, v := range opts.Tags {
		args = append(args, fmt.Sprintf("--tags=%s:%s", k, v))
	}

	for _, p := range opts.Pins {
		args = append(args, "--pin="+p)
	}

	if opts.Description != "" {
		args = append(args, "--description="+opts.Description)
	}

	buffer := bytes.NewBuffer(opts.Content)

	snapshotCreateCommand := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	snapshotCreateCommand.Stdin = buffer
	snapshotCreateCommand.Env = s.kopiaEnvironmentVariables()

	contextLogger.Info("Saving Kopia snapshot", "args", snapshotCreateCommand.Args)
	if err := RunWithLogCapture(ctx, snapshotCreateCommand, nil); err != nil {
		return fmt.Errorf("while executing Kopia command: %w", err)
	}

	return nil
}

// VerifySnapshots verifies snapshots integrity by targeting directly their root object IDs.
// It uses --json output to distinguish corruption (errorCount > 0) from
// infrastructure errors (command failed but no corruption detected).
func (s *Client) VerifySnapshots(ctx context.Context, objectIDs ...string) (VerifyResult, error) {
	contextLogger := log.FromContext(ctx)

	args := buildVerifyArgs(s.ConfigFile, objectIDs...)

	contextLogger.Info("Verifying Kopia snapshots", "args", args)

	var stdout bytes.Buffer

	verifyCmd := exec.CommandContext(ctx, s.KopiaBinary, args...) //nolint:gosec
	verifyCmd.Env = s.kopiaEnvironmentVariables()

	if err := RunWithLogCapture(ctx, verifyCmd, &stdout); err != nil {
		return parseVerifyOutput(stdout.Bytes()), fmt.Errorf("while verifying Kopia snapshots: %w", err)
	}

	return parseVerifyOutput(stdout.Bytes()), nil
}

// buildVerifyArgs builds the "kopia snapshot verify" arguments.
func buildVerifyArgs(configFile string, objectIDs ...string) []string {
	args := make([]string, 0, 5+len(objectIDs))
	args = append(args,
		"snapshot",
		"verify",
		"--json",
		"--disable-file-logging",
		"--config-file="+configFile,
	)

	for _, id := range objectIDs {
		args = append(args, "--file-id="+id)
	}

	return args
}

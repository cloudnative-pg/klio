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
	pinned bool,
) error {
	tags := map[string]string{
		klioclient.BackupContentTagName:  "tablespace",
		klioclient.TablespaceNameTagName: path.Base(tbl.Name),
		klioclient.BackupNameTagName:     backupName,
	}

	var pins []string
	if pinned {
		pins = []string{klioclient.Tier2Pin}
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   tbl.Path,
		Tags:        tags,
		Description: fmt.Sprintf("tablespace %s (%v)", tbl.Name, tbl.Oid),
		Pins:        pins,
	})
	if err != nil {
		return fmt.Errorf("unable to snapshot directory: %w", err)
	}

	return nil
}

// UploadPgData implements common.BackupUploader.
func (s *Connection) UploadPgData(
	ctx context.Context,
	backupName, pgData string,
	pinned bool,
) error {
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

	var pins []string
	if pinned {
		pins = []string{klioclient.Tier2Pin}
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   pgData,
		Tags:        tags,
		Description: "pgdata",
		Pins:        pins,
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
func (s *Connection) UploadControlFile(
	ctx context.Context,
	backupName, controlDataFileName string,
	pinned bool,
) error {
	tags := map[string]string{
		klioclient.BackupNameTagName:    backupName,
		klioclient.BackupContentTagName: "controldata",
	}

	var pins []string
	if pinned {
		pins = []string{klioclient.Tier2Pin}
	}

	err := s.kopia.SnapshotDirectory(ctx, kopia.SnapshotDirectoryOptions{
		Directory:   controlDataFileName,
		Tags:        tags,
		Description: "control data file",
		Pins:        pins,
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
	pinned bool,
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

	var pins []string
	if pinned {
		pins = []string{klioclient.Tier2Pin}
	}

	opts := kopia.SnapshotFileContentOptions{
		Content:       metadataContent,
		FileName:      "metadata.json",
		DirectoryName: fakeMetadataDirectory,
		Description:   "metadata for " + backupName,
		Tags:          tags,
		Pins:          pins,
	}

	if err := s.kopia.SnapshotFileContent(ctx, opts); err != nil {
		return fmt.Errorf("error file snapshotting metadata: %w", err)
	}

	return nil
}

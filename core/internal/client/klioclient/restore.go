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

package klioclient

import (
	"context"
	"fmt"
	"os"
	"path"
	"slices"
	"sort"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/fileutils"
)

// backupLabelFileName is the file name where the backup label should be stored.
const backupLabelFileName = "backup_label"

// RestoreExecutor guides the execution of a restore process, delegating
// the download of data to the underlying implementation.
type RestoreExecutor struct {
	restorer      BackupRestoreSupport
	configuration RestoreConfiguration
}

// RestoreConfiguration are the configuration that should be used by the restore
// process.
type RestoreConfiguration struct {
	// Name is the name of the backup that should be restored.
	Name string

	// PgDataDirectory is the target PGDATA where the files will be stored.
	PgDataDirectory string

	// TablespacesDirectory allow the user to customize the location
	// that will be used to download the tablespaces.
	TablespacesDirectory map[string]string
}

// NewRestoreExecutor creates a new restore executor given
// a certain implementation.
func NewRestoreExecutor(restorer BackupRestoreSupport, conf RestoreConfiguration) *RestoreExecutor {
	return &RestoreExecutor{
		restorer:      restorer,
		configuration: conf,
	}
}

// Restore handles the restore process.
func (r *RestoreExecutor) Restore(ctx context.Context, hostname string, destinationPath string) error {
	meta, err := r.restorer.GetMetadata(ctx, hostname, r.configuration.Name)
	if err != nil {
		return fmt.Errorf("while getting metadata for backup %s: %w", r.configuration.Name, err)
	}

	// Restore the tablespaces
	for _, tbl := range meta.Tablespaces {
		if err := r.restoreTablespace(ctx, meta, tbl); err != nil {
			return fmt.Errorf("while restoring tablespace %s: %w", tbl.Name, err)
		}
	}

	// Restore PGDATA
	if err := r.restorer.RestorePgData(ctx, meta, destinationPath); err != nil {
		return fmt.Errorf("while restoring pgdata to %s: %w", destinationPath, err)
	}

	// Restore control data file
	controlDataFileName := path.Join(destinationPath, controlDataPath)
	if err := r.restorer.RestoreControlData(ctx, meta, controlDataFileName); err != nil {
		return fmt.Errorf("while restoring control data file to %s: %w", controlDataFileName, err)
	}

	// Restore backup label
	backupLabel := path.Join(destinationPath, backupLabelFileName)
	if err := os.WriteFile(backupLabel, []byte(meta.BackupLabel), 0o600); err != nil {
		return fmt.Errorf("while writing backup label %s file: %w", backupLabel, err)
	}

	// Ensure pg_wal exists, since it is ignored by the backup.
	if err := fileutils.EnsureDirectoryExists(path.Join(destinationPath, "pg_wal")); err != nil {
		return fmt.Errorf("while creating pg_wal directory: %w", err)
	}

	return nil
}

func (r *RestoreExecutor) restoreTablespace(ctx context.Context, meta *BackupMetadata, tbl TablespaceLayout) error {
	tablespaceDestinationPath := tbl.Path
	if v, ok := r.configuration.TablespacesDirectory[tbl.Name]; ok {
		tablespaceDestinationPath = v
	}

	if err := r.restorer.RestoreTablespace(ctx, meta, tbl, tablespaceDestinationPath); err != nil {
		return fmt.Errorf("while restoring tablespace %s to %s: %w", tbl.Name, tablespaceDestinationPath, err)
	}

	return nil
}

// SortByAscendingTime sorts the backup list so that the oldest backup
// is the first on the list and the most recent one is the last on the list.
func (l BackupList) SortByAscendingTime() {
	if len(l) == 0 {
		return
	}

	sort.Slice(l, func(i, j int) bool {
		return l[i].StartedAt < l[j].StartedAt
	})
}

// GetLatestBackup gets the latest backup.
func (l BackupList) GetLatestBackup() *BackupMetadata {
	if len(l) == 0 {
		return nil
	}

	l.SortByAscendingTime()

	return &l[len(l)-1]
}

// FindClosestBackup finds the most recent backup that was taken before
// the chosen point in time.
func (l BackupList) FindClosestBackup(t time.Time) *BackupMetadata {
	if len(l) == 0 {
		return nil
	}

	l.SortByAscendingTime()
	for i, v := range slices.Backward(l) {
		if v.StoppedAt <= t.Unix() {
			return &l[i]
		}
	}

	return nil
}

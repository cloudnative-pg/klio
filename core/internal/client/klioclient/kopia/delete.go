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
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// ErrBackupNotFound is returned when attempting to delete a backup that does not exist.
var ErrBackupNotFound = errors.New("backup not found")

// deleteBackupAttempts bounds how many times DeleteBackup re-resolves the
// snapshots of a backup before giving up.
const deleteBackupAttempts = 3

// snapshotStore is the subset of the Kopia client that DeleteBackup needs.
type snapshotStore interface {
	ListSnapshots(ctx context.Context, tags map[string]string, logFn kopia.LogFunc) ([]kopia.Manifest, error)
	DeleteSnapshot(ctx context.Context, id string) error
}

// DeleteBackup removes all snapshots associated with the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	return deleteBackupSnapshots(ctx, s.kopia, hostname, name)
}

// deleteBackupSnapshots removes every snapshot of a backup on the given host.
//
// Snapshots are deleted by manifest ID, which post-backup maintenance can
// rewrite underneath us: "kopia snapshot pin" replaces a snapshot's manifest
// with a new ID, and deleting a replaced ID matches nothing and fails, leaving
// the backup in place. Deleting by root object ID is not an option, because
// unchanged content dedupes to the same root across backups and Kopia deletes
// every snapshot matching the ID it is given. Instead, resolve the snapshots
// again and retry: each attempt lists the current IDs, and snapshots deleted by
// an earlier attempt are simply no longer listed.
func deleteBackupSnapshots(ctx context.Context, store snapshotStore, hostname, name string) error {
	contextLogger := log.FromContext(ctx)

	var deleted int

	var lastErr error

	for attempt := 1; attempt <= deleteBackupAttempts; attempt++ {
		// List all snapshots for this backup (all content types)
		entries, err := store.ListSnapshots(ctx, map[string]string{
			klioclient.BackupNameTagName: name,
		}, contextLogger.Debug)
		if err != nil {
			return fmt.Errorf("while listing snapshots: %w", err)
		}

		var pending int

		var attemptErr error

		for _, entry := range entries {
			if entry.Source.Host != hostname {
				continue
			}

			pending++

			contextLogger.Info("DeleteBackup: deleting snapshot", "snapshotID", entry.ID)
			if deleteErr := store.DeleteSnapshot(ctx, entry.ID); deleteErr != nil {
				attemptErr = errors.Join(attemptErr, deleteErr)

				continue
			}

			deleted++
		}

		// Nothing left to delete: either we removed everything, or the backup
		// was not there to begin with.
		if pending == 0 {
			if deleted > 0 {
				return nil
			}

			return fmt.Errorf("%w: %s", ErrBackupNotFound, name)
		}

		if attemptErr == nil {
			return nil
		}

		lastErr = attemptErr

		contextLogger.Info("DeleteBackup: retrying with freshly resolved snapshot IDs",
			"backupName", name, "attempt", attempt, "error", attemptErr)
	}

	return lastErr
}

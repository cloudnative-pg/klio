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
	"slices"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// ErrBackupNotFound is returned when attempting to delete a backup that does not exist.
var ErrBackupNotFound = errors.New("backup not found")

// deleteBackupBackoff defines the backoff between failed attempts to delete snapshots of a backup.
//
//nolint:gochecknoglobals
var deleteBackupBackoff = wait.Backoff{
	Duration: time.Second,
	Factor:   2,
	Cap:      2 * time.Second,
	Steps:    3,
}

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
func deleteBackupSnapshots(ctx context.Context, store snapshotStore, hostname, name string) error {
	contextLogger := log.FromContext(ctx)

	// Concurrent operations on the same snapshots (e.g. post-backup
	// maintenance) can invalidate the IDs resolved below before they are
	// used, so this race condition is tolerated with retries.
	retryErr := wait.ExponentialBackoffWithContext(ctx, deleteBackupBackoff,
		func(ctx context.Context) (bool, error) {
			entries, err := store.ListSnapshots(ctx, map[string]string{
				klioclient.BackupNameTagName: name,
			}, contextLogger.Debug)
			if err != nil {
				return false, fmt.Errorf("while listing snapshots: %w", err)
			}

			entries = slices.DeleteFunc(entries, func(e kopia.Manifest) bool {
				return e.Source.Host != hostname
			})

			if len(entries) == 0 {
				return false, fmt.Errorf("%w: %s", ErrBackupNotFound, name)
			}

			err = deleteSnapshots(ctx, store, entries)
			if err == nil {
				return true, nil
			}

			contextLogger.Info("DeleteBackup: an error occurred while trying to delete backup's snapshots, retrying...",
				"backupName", name, "error", err)

			return false, nil
		},
	)

	return retryErr
}

// deleteSnapshots deletes every given entry, joining and returning any
// deletion errors.
func deleteSnapshots(
	ctx context.Context,
	store snapshotStore,
	entries []kopia.Manifest,
) error {
	contextLogger := log.FromContext(ctx)

	var err error

	for _, entry := range entries {
		contextLogger.Info("DeleteBackup: deleting snapshot", "snapshotID", entry.ID)
		if deleteErr := store.DeleteSnapshot(ctx, entry.ID); deleteErr != nil {
			err = errors.Join(err, deleteErr)
		}
	}

	return err
}

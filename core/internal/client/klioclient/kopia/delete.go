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
)

// ErrBackupNotFound is returned when attempting to delete a backup that does not exist.
var ErrBackupNotFound = errors.New("backup not found")

// DeleteBackup removes all snapshots associated with the backup with the provided name.
func (s *Connection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	contextLogger := log.FromContext(ctx)

	// List all snapshots for this backup (all content types)
	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: name,
	}, contextLogger.Debug)
	if err != nil {
		return fmt.Errorf("while listing snapshots: %w", err)
	}

	var deleted int
	for _, entry := range entries {
		if entry.Source.Host == hostname {
			contextLogger.Info("DeleteBackup: deleting snapshot", "snapshotID", entry.ID)
			if deleteErr := s.kopia.DeleteSnapshot(ctx, entry.ID); deleteErr != nil {
				err = errors.Join(err, deleteErr)
			} else {
				deleted++
			}
		}
	}

	if err != nil {
		return err
	}

	if deleted == 0 {
		return fmt.Errorf("%w: %s", ErrBackupNotFound, name)
	}

	return nil
}

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
	"fmt"

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

	return s.kopia.RestoreSnapshot(ctx, source, destinationDirectory)
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

	return s.kopia.RestoreSnapshot(ctx, source, destinationDirectory)
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

	return s.kopia.RestoreSnapshot(ctx, source, destinationPath)
}

func (s *Connection) getSnapshotID(
	ctx context.Context,
	tags map[string]string,
) (string, error) {
	contextLogger := log.FromContext(ctx)
	entries, err := s.kopia.ListSnapshots(ctx, tags, contextLogger.Info)
	if err != nil {
		return "", fmt.Errorf("while executing Kopia command: %w", err)
	}

	for _, entry := range entries {
		if s.GetHostname() != "" && entry.Source.Host == s.GetHostname() {
			return entry.ID, nil
		}
	}

	return "", newNoSnapshotFound(s.GetHostname(), tags)
}

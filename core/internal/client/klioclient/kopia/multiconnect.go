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

	"github.com/cloudnative-pg/machinery/pkg/stringset"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

const (
	tier1AnnotationName    = "klio.io/tier1"
	tier2AnnotationName    = "klio.io/tier2"
	presentAnnotationValue = "present"
)

// ErrUnsupportedWriteOperation is raised when a write operation is attempted against
// a client having only tier2.
var ErrUnsupportedWriteOperation = errors.New("write operations require tier1 connection (server is read-only)")

// MultiConnection is composed by two clients: one for tier1
// and one for tier2.
type MultiConnection struct {
	Tier1 klioclient.Client
	Tier2 klioclient.Client
}

// MultiConnect creates two connections: one to tier1 and one to
// tier2 (optional). Writes are directly routed to tier1. Reads
// are tried against tier1 and, if the required information has
// not been found, they are tried against tier2.
func MultiConnect(
	ctx context.Context,
	clientConfig *config.ClientConfig,
) (*MultiConnection, error) {
	var mc MultiConnection

	if clientConfig.Base.URL != "" {
		tier1, err := ConnectTier1(ctx, clientConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to tier1: %w", err)
		}
		mc.Tier1 = tier1
	}

	if clientConfig.Base.Tier2URL != "" {
		tier2, err := ConnectTier2(ctx, clientConfig)
		if err != nil {
			return nil, fmt.Errorf("connecting to tier2: %w", err)
		}
		mc.Tier2 = tier2
	}

	if mc.Tier1 == nil && mc.Tier2 == nil {
		return nil, errors.New("at least one of tier1 and tier2 is required")
	}

	return &mc, nil
}

// Close closes the connections to tier1 and tier2 repositories and cleans up temporary files.
func (s *MultiConnection) Close(ctx context.Context) {
	if s.Tier1 != nil {
		s.Tier1.Close(ctx)
	}

	if s.Tier2 != nil {
		s.Tier2.Close(ctx)
	}
}

// GetUsername implements the Client interface.
func (s *MultiConnection) GetUsername() string {
	return s.getReadClient().GetUsername()
}

// GetHostname implements the Client interface.
func (s *MultiConnection) GetHostname() string {
	return s.getReadClient().GetHostname()
}

// DeleteBackup implements the Client interface.
func (s *MultiConnection) DeleteBackup(ctx context.Context, hostname string, name string) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.DeleteBackup(ctx, hostname, name)
}

// SetRetentionPolicy implements the Client interface.
func (s *MultiConnection) SetRetentionPolicy(
	ctx context.Context,
	t kopia.Target,
	p kopia.RetentionPolicy,
) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.SetRetentionPolicy(ctx, t, p)
}

// GetRetentionPolicy implements the Client interface.
func (s *MultiConnection) GetRetentionPolicy(
	ctx context.Context,
	t kopia.Target,
) (*kopia.RetentionPolicy, error) {
	return s.getReadClient().GetRetentionPolicy(ctx, t)
}

// GetMetadata implements the BackupRestoreSupport interface.
func (s *MultiConnection) GetMetadata(
	ctx context.Context,
	hostname string,
	name string,
) (*klioclient.BackupMetadata, error) {
	if s.Tier1 != nil {
		meta, err := s.Tier1.GetMetadata(ctx, hostname, name)
		if err == nil {
			return markTier1(meta), nil
		}

		var noBackup NoBackupFoundError
		if !errors.As(err, &noBackup) {
			return nil, fmt.Errorf("while getting metadata from tier1: %w", err)
		}
	}

	if s.Tier2 == nil {
		return nil, newNoBackupFoundError(hostname, name)
	}

	meta, err := s.Tier2.GetMetadata(ctx, hostname, name)
	if err != nil {
		return nil, fmt.Errorf("while getting metadata from tier2: %w", err)
	}

	return markTier2(meta), nil
}

// ApplyRetentionPolicy implements the Client interface.
func (s *MultiConnection) ApplyRetentionPolicy(ctx context.Context, t kopia.Target) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.ApplyRetentionPolicy(ctx, t)
}

// ListBackups implements the BackupRestoreSupport interface.
//
//nolint:cyclop
func (s *MultiConnection) ListBackups(
	ctx context.Context,
	hostname string,
) (klioclient.BackupList, error) {
	var tier1List, tier2List klioclient.BackupList
	var err error

	if s.Tier1 != nil {
		tier1List, err = s.Tier1.ListBackups(ctx, hostname)
		if err != nil {
			return nil, fmt.Errorf("while listing backups from tier1: %w", err)
		}
	}

	if s.Tier2 != nil {
		tier2List, err = s.Tier2.ListBackups(ctx, hostname)
		if err != nil {
			return nil, fmt.Errorf("while listing backups from tier2: %w", err)
		}
	}

	tier1BackupNames := stringset.New()
	for i := range tier1List {
		tier1BackupNames.Put(tier1List[i].Name)
	}

	tier2BackupNames := stringset.New()
	for i := range tier2List {
		tier2BackupNames.Put(tier2List[i].Name)
	}

	result := make(klioclient.BackupList, 0, len(tier1List)+len(tier2List))

	for i := range tier1List {
		markTier1(&tier1List[i])

		if tier2BackupNames.Has(tier1List[i].Name) {
			markTier2(&tier1List[i])
		}

		result = append(result, tier1List[i])
	}

	for i := range tier2List {
		markTier2(&tier2List[i])

		if tier1BackupNames.Has(tier2List[i].Name) {
			// This backup has been put in the backup list
			// by the previous loop
			continue
		}

		result = append(result, tier2List[i])
	}

	return result, nil
}

// RestoreTablespace implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestoreTablespace(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	tbl klioclient.TablespaceLayout,
	destinationDirectory string,
) error {
	return s.getClientFromMetadata(metadata).RestoreTablespace(ctx, metadata, tbl, destinationDirectory)
}

// RestorePgData implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestorePgData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationDirectory string,
) error {
	return s.getClientFromMetadata(metadata).RestorePgData(ctx, metadata, destinationDirectory)
}

// RestoreControlData implements the BackupRestoreSupport interface.
func (s *MultiConnection) RestoreControlData(
	ctx context.Context,
	metadata *klioclient.BackupMetadata,
	destinationPath string,
) error {
	return s.getClientFromMetadata(metadata).RestoreControlData(ctx, metadata, destinationPath)
}

// UploadTablespace implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadTablespace(
	ctx context.Context,
	backupName string,
	tbl klioclient.TablespaceLayout,
	pinned bool,
) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.UploadTablespace(ctx, backupName, tbl, pinned)
}

// UploadPgData implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadPgData(
	ctx context.Context,
	backupName string,
	pgData string,
	pinned bool,
) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.UploadPgData(ctx, backupName, pgData, pinned)
}

// UploadControlFile implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadControlFile(
	ctx context.Context,
	backupName string,
	controlDataFileName string,
	pinned bool,
) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.UploadControlFile(ctx, backupName, controlDataFileName, pinned)
}

// UploadBackupMetadata implements the BackupExecutorSupport interface.
func (s *MultiConnection) UploadBackupMetadata(
	ctx context.Context,
	backupName string,
	metadata *klioclient.BackupMetadata,
	pinned bool,
) error {
	if s.Tier1 == nil {
		return ErrUnsupportedWriteOperation
	}

	return s.Tier1.UploadBackupMetadata(ctx, backupName, metadata, pinned)
}

func (s *MultiConnection) getClientFromMetadata(meta *klioclient.BackupMetadata) klioclient.Client {
	if meta.Annotations[tier1AnnotationName] == presentAnnotationValue {
		return s.Tier1
	}

	if meta.Annotations[tier2AnnotationName] == presentAnnotationValue {
		return s.Tier2
	}

	return nil
}

func markTier1(meta *klioclient.BackupMetadata) *klioclient.BackupMetadata {
	meta.SetAnnotation(tier1AnnotationName, presentAnnotationValue)

	return meta
}

func markTier2(meta *klioclient.BackupMetadata) *klioclient.BackupMetadata {
	meta.SetAnnotation(tier2AnnotationName, presentAnnotationValue)

	return meta
}

func (s *MultiConnection) getReadClient() klioclient.Client {
	if s.Tier1 != nil {
		return s.Tier1
	}

	if s.Tier2 != nil {
		return s.Tier2
	}

	return nil
}

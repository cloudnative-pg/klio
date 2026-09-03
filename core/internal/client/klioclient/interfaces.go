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

	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

// BackupExecutorSupport contains the functions needed to execute a backup.
type BackupExecutorSupport interface {
	// UploadTablespace uploads the tablespace with the passed layout to
	// the backup store.
	UploadTablespace(ctx context.Context, backupName string, tbl TablespaceLayout, pinned bool) error

	// UploadPgData uploads the PGData to the backup store.
	UploadPgData(ctx context.Context, backupName string, pgData string, pinned bool) error

	// UploadControlFile uploads the control file to the backup store.
	UploadControlFile(ctx context.Context, backupName string, controlDataFileName string, pinned bool) error

	// UploadBackupMetadata is called to upload the control file and to mark a backup successfully done.
	UploadBackupMetadata(ctx context.Context, backupName string, metadata *BackupMetadata, pinned bool) error
}

// BackupRestoreSupport contains the functions needed to restore a backup.
type BackupRestoreSupport interface {
	// ListBackups list all the backups in the repository.
	ListBackups(ctx context.Context, hostname string) (BackupList, error)

	// GetMetadata implements the RestoreExecutor interface.
	GetMetadata(ctx context.Context, hostname string, name string) (*BackupMetadata, error)

	// RestoreTablespace restores the passed tablespace in the specified
	// folder.
	RestoreTablespace(
		ctx context.Context,
		metadata *BackupMetadata,
		tbl TablespaceLayout,
		destinationDirectory string,
	) error

	// RestorePgData restores the passed pgdata in the specified
	// directory.
	RestorePgData(ctx context.Context, metadata *BackupMetadata, destinationDirectory string) error

	// RestoreControlData restores the pg control data file into
	// the passed file name.
	RestoreControlData(ctx context.Context, metadata *BackupMetadata, destinationPath string) error
}

// Client represents a Kopia client.
type Client interface {
	BackupExecutorSupport
	BackupRestoreSupport

	// DeleteBackup removes the backup with the provided name.
	DeleteBackup(ctx context.Context, hostname string, name string) error

	// GetUsername gets the username we are using for the connection.
	// This is read from the client certificate.
	GetUsername() string

	// GetHostname gets the hostname we are using for the connection.
	// This is read from the client certificate.
	GetHostname() string

	// SetRetentionPolicy sets the retention policy for backups of this cluster.
	SetRetentionPolicy(ctx context.Context, t kopia.Target, p kopia.RetentionPolicy) error

	// GetRetentionPolicy gets the currently applied retention policy for this cluster.
	GetRetentionPolicy(ctx context.Context, t kopia.Target) (*kopia.RetentionPolicy, error)

	// ApplyRetentionPolicy applies the retention policy for this cluster, deleting any
	// snapshots that are no longer needed.
	ApplyRetentionPolicy(ctx context.Context, t kopia.Target) error

	// Close closes the underlying connection.
	Close(ctx context.Context)
}

// WALUploader is the underlying implementation of a WAL
// uploader.
type WALUploader interface {
	// SendBlock sends a WAL Block.
	SendBlock(ctx context.Context, block []byte) error

	// Close closes the WAL streaming session
	Close(ctx context.Context) error
}

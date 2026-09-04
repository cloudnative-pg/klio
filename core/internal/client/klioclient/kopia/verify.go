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
	kopiaClient "github.com/cloudnative-pg/klio/core/internal/kopia"
)

// ErrNoSnapshotsForBackup is returned when no verifiable snapshot can be
// found for a backup name.
var ErrNoSnapshotsForBackup = errors.New("no snapshots found for backup")

// BackupVerificationError is returned when backup verification detects corruption.
type BackupVerificationError struct {
	// Result contains the verification details from Kopia.
	Result kopiaClient.VerifyResult
	// Err is the underlying error.
	Err error
}

func (e *BackupVerificationError) Error() string {
	return fmt.Sprintf("backup verification failed with %d corrupt object(s): %v", e.Result.ErrorCount, e.Err)
}

func (e *BackupVerificationError) Unwrap() error {
	return e.Err
}

// VerifyOpts configures which backups to verify.
type VerifyOpts struct {
	// Hostname is the cluster hostname to verify backups for.
	Hostname string

	// BackupNames specifies which backups to verify (ignored if All is true).
	BackupNames []string

	// All, if true, verifies all backups for the hostname.
	All bool
}

// VerifyBackups verifies the integrity of the specified backups.
// If opts.All is true, all backups for the hostname are verified.
// Otherwise, only the backups in opts.BackupNames are verified.
func (s *Connection) VerifyBackups(ctx context.Context, opts VerifyOpts) error {
	if opts.All {
		return s.verifyAllBackups(ctx, opts.Hostname)
	}

	return s.verifySpecificBackups(ctx, opts.Hostname, opts.BackupNames)
}

// verifyAllBackups verifies all backups for the given hostname.
// It directly invokes kopia snapshot verify without fetching snapshot IDs,
// relying on the fact that the kopia client is already scoped to the hostname.
func (s *Connection) verifyAllBackups(ctx context.Context, hostname string) error {
	contextLogger := log.FromContext(ctx)

	// When verifying all backups, we can skip fetching snapshot IDs.
	// The kopia client is already scoped to the correct hostname via
	// --override-hostname, so `kopia snapshot verify` without IDs will
	// verify all snapshots for this hostname.
	contextLogger.Info("Verifying all backups for hostname", "hostname", hostname)

	result, err := s.kopia.VerifySnapshots(ctx)
	if err != nil {
		return classifyVerifyError(ctx, result, err)
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// verifySpecificBackups verifies the specified backups by resolving them to the
// root object IDs of their snapshots.
func (s *Connection) verifySpecificBackups(ctx context.Context, hostname string, backupNames []string) error {
	contextLogger := log.FromContext(ctx)

	if len(backupNames) == 0 {
		return nil
	}

	var rootObjectIDs []string

	for _, name := range backupNames {
		ids, err := s.getBackupRootObjIDs(ctx, hostname, name)
		if err != nil {
			return fmt.Errorf("backup %q: %w", name, err)
		}

		rootObjectIDs = append(rootObjectIDs, ids...)
	}

	contextLogger.Info("Verifying backups", "backupNames", backupNames, "objectsCount", len(rootObjectIDs))

	result, err := s.kopia.VerifySnapshots(ctx, rootObjectIDs...)
	if err != nil {
		return classifyVerifyError(ctx, result, err)
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// getBackupRootObjIDs resolves a backup name to the root object IDs of its
// constituent Kopia snapshots.
func (s *Connection) getBackupRootObjIDs(
	ctx context.Context,
	hostname, backupName string,
) ([]string, error) {
	contextLogger := log.FromContext(ctx)

	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: backupName,
	}, contextLogger.Debug)
	if err != nil {
		return nil, err
	}

	rootObjIDs := make([]string, 0, len(entries))

	for _, e := range entries {
		if e.Source.Host != hostname {
			continue
		}

		// An incomplete snapshot has no root entry to verify.
		if e.RootEntry == nil || e.RootEntry.ObjID == "" {
			contextLogger.Info("Skipping snapshot without a root object ID",
				"backupName", backupName, "snapshotID", e.ID)

			continue
		}

		rootObjIDs = append(rootObjIDs, e.RootEntry.ObjID)
	}

	if len(rootObjIDs) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoSnapshotsForBackup, backupName)
	}

	return rootObjIDs, nil
}

// classifyVerifyError inspects the verify result to distinguish corruption
// (errorCount > 0) from infrastructure errors.
func classifyVerifyError(ctx context.Context, result kopiaClient.VerifyResult, err error) error {
	contextLogger := log.FromContext(ctx)

	if result.ErrorCount > 0 {
		contextLogger.Error(err, "Backup verification detected corruption",
			"errorCount", result.ErrorCount,
			"errors", result.ErrorStrings,
		)

		return &BackupVerificationError{
			Result: result,
			Err:    err,
		}
	}

	return fmt.Errorf("backup verification encountered an infrastructure error: %w", err)
}

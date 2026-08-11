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

var (
	// ErrNoSnapshotsForBackup is returned when no verifiable snapshot can be
	// found for a backup name.
	ErrNoSnapshotsForBackup = errors.New("no snapshots found for backup")

	// ErrUnsupportedRootEntryType is returned when a snapshot's root entry is
	// neither a directory nor a file, so it cannot be verified by object ID.
	ErrUnsupportedRootEntryType = errors.New("unsupported snapshot root entry type")
)

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

	result, err := s.kopia.VerifySnapshots(ctx, kopiaClient.VerifySnapshotsOptions{})
	if err != nil {
		return classifyVerifyError(ctx, result, err)
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// verifySpecificBackups verifies the specified backups by resolving them to the
// root object IDs of their snapshots.
//
// A backup whose snapshots fail to resolve does not stop the others from being
// verified: their resolution errors are joined and returned alongside whatever
// verification result the successfully resolved backups produce.
func (s *Connection) verifySpecificBackups(ctx context.Context, hostname string, backupNames []string) error {
	contextLogger := log.FromContext(ctx)

	var verifyOpts kopiaClient.VerifySnapshotsOptions

	var resolveErr error

	for _, name := range backupNames {
		opts, err := s.getRootObjectsForBackup(ctx, hostname, name)
		if err != nil {
			resolveErr = errors.Join(resolveErr, fmt.Errorf("backup %q: %w", name, err))

			continue
		}

		verifyOpts.DirectoryIDs = append(verifyOpts.DirectoryIDs, opts.DirectoryIDs...)
		verifyOpts.FileIDs = append(verifyOpts.FileIDs, opts.FileIDs...)
	}

	if verifyOpts.IsEmpty() {
		return resolveErr
	}

	contextLogger.Info("Verifying backups", "backupNames", backupNames, "snapshotCount", verifyOpts.Len())

	result, err := s.kopia.VerifySnapshots(ctx, verifyOpts)
	if err != nil {
		return errors.Join(resolveErr, classifyVerifyError(ctx, result, err))
	}

	if resolveErr != nil {
		return resolveErr
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// getRootObjectsForBackup resolves a backup name to the root objects of its
// constituent Kopia snapshots, split by object kind.
//
// The root object ID is used rather than the snapshot manifest ID because it is
// a stable identity. Post-backup maintenance unpins the snapshots of a backup
// that reached tier2, and "kopia snapshot pin" rewrites the manifest under a
// new ID, so a manifest ID resolved here can already have been replaced by the
// time verification runs. The root object ID is untouched by that rewrite.
//
// A backup mixes both kinds: pgdata and metadata are directory snapshots, while
// the control data file is snapshotted on its own and so has a file root.
func (s *Connection) getRootObjectsForBackup(
	ctx context.Context,
	hostname, backupName string,
) (kopiaClient.VerifySnapshotsOptions, error) {
	contextLogger := log.FromContext(ctx)

	var opts kopiaClient.VerifySnapshotsOptions

	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: backupName,
	}, contextLogger.Debug)
	if err != nil {
		return opts, err
	}

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

		switch e.RootEntry.Type {
		case kopiaClient.EntryTypeDirectory:
			opts.DirectoryIDs = append(opts.DirectoryIDs, e.RootEntry.ObjID)
		case kopiaClient.EntryTypeFile:
			opts.FileIDs = append(opts.FileIDs, e.RootEntry.ObjID)
		default:
			// Verifying with the wrong flag reports a healthy object as
			// corrupt, so refuse rather than guess.
			return opts, fmt.Errorf("%w: snapshot %q has root entry type %q",
				ErrUnsupportedRootEntryType, e.ID, e.RootEntry.Type)
		}
	}

	if opts.IsEmpty() {
		return opts, fmt.Errorf("%w: %q", ErrNoSnapshotsForBackup, backupName)
	}

	return opts, nil
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

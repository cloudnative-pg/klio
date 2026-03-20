package kopia

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	kopiaClient "github.com/cloudnative-pg/klio/core/internal/kopia"
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

	result, err := s.kopia.VerifySnapshots(ctx)
	if err != nil {
		return classifyVerifyError(ctx, result, err)
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// verifySpecificBackups verifies the specified backups by resolving their snapshot IDs.
func (s *Connection) verifySpecificBackups(ctx context.Context, hostname string, backupNames []string) error {
	contextLogger := log.FromContext(ctx)

	var allSnapshotIDs []string
	for _, name := range backupNames {
		ids, err := s.getSnapshotIDsForBackup(ctx, hostname, name)
		if err != nil {
			return fmt.Errorf("backup %q: %w", name, err)
		}
		allSnapshotIDs = append(allSnapshotIDs, ids...)
	}

	contextLogger.Info("Verifying backups", "backupNames", backupNames, "snapshotCount", len(allSnapshotIDs))

	result, err := s.kopia.VerifySnapshots(ctx, allSnapshotIDs...)
	if err != nil {
		return classifyVerifyError(ctx, result, err)
	}

	contextLogger.Info("All backups verified successfully")

	return nil
}

// getSnapshotIDsForBackup resolves a backup name to its constituent Kopia snapshot IDs.
func (s *Connection) getSnapshotIDsForBackup(ctx context.Context, hostname, backupName string) ([]string, error) {
	contextLogger := log.FromContext(ctx)

	entries, err := s.kopia.ListSnapshots(ctx, map[string]string{
		klioclient.BackupNameTagName: backupName,
	}, contextLogger.Debug)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, e := range entries {
		if e.Source.Host == hostname {
			ids = append(ids, e.ID)
		}
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("no snapshots found for backup %q", backupName)
	}

	return ids, nil
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

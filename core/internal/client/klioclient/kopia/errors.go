package kopia

import "fmt"

// NoBackupFoundError is raised when the requested backup has not been found
// in the Kopia manifests store.
type NoBackupFoundError struct {
	backupName string
}

func newNoBackupFoundError(backupName string) NoBackupFoundError {
	return NoBackupFoundError{
		backupName: backupName,
	}
}

func (err NoBackupFoundError) Error() string {
	return fmt.Sprintf("backup %v not found", err.backupName)
}

// NoSnapshotFoundError is raised when the requested snapshot has not been found
// in the Kopia manifests store.
type NoSnapshotFoundError struct {
	hostname string
	tags     map[string]string
}

func newNoSnapshotFound(hostname string, tags map[string]string) NoSnapshotFoundError {
	return NoSnapshotFoundError{
		hostname: hostname,
		tags:     tags,
	}
}

func (err NoSnapshotFoundError) Error() string {
	return fmt.Sprintf("snapshot not found for tags %q and hostname %q", err.tags, err.hostname)
}

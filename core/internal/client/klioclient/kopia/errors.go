package kopia

import "fmt"

// NoBackupFoundError is raised when the requested backup has not been found
// in the Kopia manifests store.
type NoBackupFoundError struct {
	hostName   string
	backupName string
}

func newNoBackupFoundError(hostName string, backupName string) NoBackupFoundError {
	return NoBackupFoundError{
		hostName:   hostName,
		backupName: backupName,
	}
}

func (err NoBackupFoundError) Error() string {
	return fmt.Sprintf("backup %q for host %q not found", err.backupName, err.hostName)
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

package kopia

import "fmt"

type multipleBackupsFoundError struct {
	backupName string
	quantity   int
}

func newMultipleBackupsFoundError(backupName string, quantity int) *multipleBackupsFoundError {
	return &multipleBackupsFoundError{
		backupName: backupName,
		quantity:   quantity,
	}
}

func (err *multipleBackupsFoundError) Error() string {
	return fmt.Sprintf(
		"found %v backups with name %v", err.quantity, err.backupName)
}

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

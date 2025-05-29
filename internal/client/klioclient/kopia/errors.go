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

type noBackupFoundError struct {
	backupName string
}

func newNoBackupFoundError(backupName string) *noBackupFoundError {
	return &noBackupFoundError{
		backupName: backupName,
	}
}

func (err *noBackupFoundError) Error() string {
	return fmt.Sprintf("backup %v not found", err.backupName)
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
)

// walSubdirectoryLength is the length of the prefix of the WAL file
// name that will be used to create the directory where the WAL
// file will be stored.
//
// As an example, with a prefix of 16 characters:
//
//	cluster-example/0000000100000000/00000001000000000000000E
const walSubdirectoryLength = 16

// expectedWalFileNameLength is the expected name of a WAL
// file.
const expectedWalFileNameLength = 24

// getWALArchivePath gets the name of the file where
// the passed WAL file will be archived.
func getWALArchivePath(clusterName, walName string) string {
	walNameWithoutExtension := strings.TrimSuffix(walName, path.Ext(walName))
	if len(walNameWithoutExtension) == expectedWalFileNameLength {
		return path.Join(clusterName, walName[0:walSubdirectoryLength], walName)
	}

	return path.Join(clusterName, walName)
}

// IsWALFileExisting checks if a WAL file exists in the repository.
func (c *Connection) IsWALFileExisting(clusterName string, walName string) (bool, error) {
	walFilePath := getWALArchivePath(clusterName, walName)

	_, err := c.fs.Stat(walFilePath)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// GetLatestWALFileForCluster gets the latest archived WAL for a certain cluster
//
//nolint:cyclop
func (c *Connection) GetLatestWALFileForCluster(
	ctx context.Context,
	clusterName string,
) (string, error) {
	logger := log.FromContext(ctx)

	readClusterDir, err := afero.ReadDir(c.fs, clusterName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		logger.Error(
			err,
			"while reading cluster directory",
			"clusterName", clusterName,
		)

		return "", fmt.Errorf("while reading cluster directory: %w", err)
	}

	var latestWalDirectoryName string
	for _, entry := range readClusterDir {
		if !entry.IsDir() {
			continue
		}

		if strings.Compare(latestWalDirectoryName, entry.Name()) == -1 {
			latestWalDirectoryName = entry.Name()
		}
	}

	if latestWalDirectoryName == "" {
		return "", nil
	}

	latestWalDirectoryName = path.Join(clusterName, latestWalDirectoryName)
	readWalDirectory, err := afero.ReadDir(c.fs, latestWalDirectoryName)
	if err != nil {
		logger.Error(err, "while reading directory", "latestWalDirectoryName", latestWalDirectoryName)
		return "", fmt.Errorf("while reading WAL directory: %w", err)
	}

	var lastWal string
	for _, entry := range readWalDirectory {
		if entry.IsDir() {
			continue
		}

		if strings.Compare(lastWal, entry.Name()) == -1 {
			lastWal = entry.Name()
		}
	}

	if lastWal == "" {
		return "", nil
	}

	return lastWal, nil
}

package repository

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"
)

// SetFirstRequiredOnCluster sets the first WAL that is required on a certain
// cluster and deletes all the WALs that are precedent to that.
func (c *Connection) SetFirstRequiredOnCluster(ctx context.Context, clusterDirectory, firstWAL string) error {
	logger := log.FromContext(ctx)
	logger.Info("Setting first required WAL", "clusterDirectory", clusterDirectory, "firstWAL", firstWAL)

	clusterPathEntries, err := afero.ReadDir(c.fs, clusterDirectory)
	if err != nil {
		return fmt.Errorf("cannot read WAL directory: %w", err)
	}

	walPrefix := firstWAL[0:walSubdirectoryLength]

	for _, entry := range clusterPathEntries {
		// We only care about directories
		if !entry.IsDir() {
			continue
		}

		baseName := path.Base(entry.Name())
		fullPath := path.Join(clusterDirectory, baseName)

		// We only care about WAL directories
		if len(baseName) != walSubdirectoryLength {
			continue
		}

		switch strings.Compare(baseName, walPrefix) {
		case -1:
			if err := setFirstRequiredOnDirectory(ctx, c.fs, fullPath, firstWAL); err != nil {
				logger.Error(
					err,
					"Error while enforcing retention policies on directory, skipping.",
					"fullPath", fullPath)
			}

		case 0:
			if err := setFirstRequiredOnDirectory(ctx, c.fs, fullPath, firstWAL); err != nil {
				logger.Error(
					err,
					"Error while enforcing retention policies on directory, skipping.",
					"fullPath", fullPath)
			}

		case 1:
			logger.Trace("Retaining WAL directory", "fullPath", fullPath)
			continue
		}
	}

	return nil
}

func setFirstRequiredOnDirectory( //nolint:cyclop
	ctx context.Context,
	fs afero.Fs,
	directory string,
	firstWAL string,
) error {
	logger := log.FromContext(ctx)

	entries, err := afero.ReadDir(fs, directory)
	if err != nil {
		return fmt.Errorf("cannot read WAL directory: %w", err)
	}

	// Step 1: remove old WAL files
	for _, entry := range entries {
		// We only care about WAL files
		if entry.IsDir() {
			continue
		}

		baseName := path.Base(entry.Name())
		fullPath := path.Join(directory, baseName)

		if err := ValidateWalFileName(baseName); err != nil {
			logger.Warning(
				"Retaining unknown file",
				"path", fullPath,
				"validationError", err.Error(),
			)

			continue
		}

		if path.Ext(baseName) != "" {
			logger.Trace(
				"Skipping potentially uncompleted WAL file",
				"path", fullPath)

			continue
		}

		if strings.Compare(baseName, firstWAL) == -1 {
			if err := fs.Remove(fullPath); err != nil {
				logger.Error(
					err,
					"Error while deleting old WAL file, skipping",
					"fullPath", fullPath)
			}
		}
	}

	// Step 2: if the directory is empty, remove it
	entries, err = afero.ReadDir(fs, directory)
	if err != nil {
		return fmt.Errorf("cannot read WAL directory: %w", err)
	}
	if len(entries) == 0 {
		if err := fs.Remove(directory); err != nil {
			logger.Error(
				err,
				"Error while deleting supposedly empty directory, skipping",
				"directory", directory)
		}
	}

	return nil
}

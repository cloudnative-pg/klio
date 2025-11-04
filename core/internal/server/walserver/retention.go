package walserver

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/spf13/afero"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// SetFirstRequiredWAL drops all the WAL older than the passed one, effectively
// applying the retention policy.
func (w *Implementation) SetFirstRequiredWAL(
	_ context.Context,
	request *grpc.SetFirstRequiredWALRequest,
) (*grpc.SetFirstRequiredWALResult, error) {
	if err := validatePathComponent(request.GetClusterName()); err != nil {
		w.logger.Warning("Wrong cluster name used in WAL SetFirstRequired",
			"clusterName", request.GetClusterName())
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := validateWalFileName(request.GetFirstRequiredWal()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", request.GetFirstRequiredWal())
	}

	if err := w.internalSetFirstRequiredOnCluster(
		w.conn.FS,
		request.GetClusterName(),
		request.GetFirstRequiredWal(),
	); err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"while enforcing first required WAL %q for cluster %q: %v",
			request.GetFirstRequiredWal(),
			request.GetClusterName(),
			err.Error(),
		)
	}

	return &grpc.SetFirstRequiredWALResult{}, nil
}

func (w *Implementation) internalSetFirstRequiredOnCluster(fs afero.Fs, clusterDirectory, firstWAL string) error {
	w.logger.Info("Setting first required WAL", "clusterDirectory", clusterDirectory, "firstWAL", firstWAL)

	clusterPathEntries, err := afero.ReadDir(fs, clusterDirectory)
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
			if err := w.internalSetFirstRequiredOnDirectory(fs, fullPath, firstWAL); err != nil {
				w.logger.Error(
					err,
					"Error while enforcing retention policies on directory, skipping.",
					"fullPath", fullPath)
			}

		case 0:
			if err := w.internalSetFirstRequiredOnDirectory(fs, fullPath, firstWAL); err != nil {
				w.logger.Error(
					err,
					"Error while enforcing retention policies on directory, skipping.",
					"fullPath", fullPath)
			}

		case 1:
			w.logger.Trace("Retaining WAL directory", "fullPath", fullPath)
			continue
		}
	}

	return nil
}

func (w *Implementation) internalSetFirstRequiredOnDirectory( //nolint:cyclop
	fs afero.Fs,
	directory string,
	firstWAL string,
) error {
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

		if err := validateWalFileName(baseName); err != nil {
			w.logger.Warning(
				"Retaining unknown file",
				"path", fullPath,
				"validationError", err.Error(),
			)

			continue
		}

		if path.Ext(baseName) != "" {
			w.logger.Trace(
				"Skipping potentially uncompleted WAL file",
				"path", fullPath)

			continue
		}

		if strings.Compare(baseName, firstWAL) == -1 {
			if err := fs.Remove(fullPath); err != nil {
				w.logger.Error(
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
			w.logger.Error(
				err,
				"Error while deleting supposedly empty directory, skipping",
				"directory", directory)
		}
	}

	return nil
}

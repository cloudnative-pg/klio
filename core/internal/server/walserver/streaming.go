package walserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// GetMetadata implements the GetMetadata GRPC call.
func (w *Implementation) GetMetadata(
	ctx context.Context,
	req *grpc.GetMetadataRequest,
) (*grpc.ClusterMetadata, error) {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	var metadata *grpc.ClusterMetadata
	var err error

	if metadata, err = w.getClusterMetadata(ctx, req.GetClusterName()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, status.Errorf(codes.NotFound, "not found")
		}

		return nil, status.Errorf(codes.Internal, "error while reading cluster metadata: %v", err.Error())
	}

	return metadata, nil
}

// RequestWALStart implements the corresponding GRPC call, validating the WAL
// streaming request of a client.
func (w *Implementation) RequestWALStart(
	ctx context.Context,
	req *grpc.RequestWALStartRequest,
) (*grpc.RequestWALStartResult, error) {
	w.logger.Info(
		"RequestWALStart, negotiating the starting point with the client",
		"cluster", req.GetClusterName(),
		"systemID", req.GetSystemId(),
		"currentWAL", req.GetCurrentWalName())

	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	// Step 1: recover cluster metadata or create new metadata for this cluster
	var metadata *grpc.ClusterMetadata
	var err error

	if metadata, err = w.getClusterMetadata(ctx, req.GetClusterName()); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, status.Errorf(codes.Internal, "error while reading cluster metadata: %v", err.Error())
		}

		metadata = &grpc.ClusterMetadata{
			SystemId: req.GetSystemId(),
		}

		if err := w.writeClusterMetadata(ctx, req.GetClusterName(), metadata); err != nil {
			return nil, status.Errorf(codes.Internal, "error while writing cluster metadata: %v", err.Error())
		}
	}

	// Step 2: ensure system ID is consistent
	if metadata.GetSystemId() != req.GetSystemId() {
		return nil, status.Errorf(codes.InvalidArgument, "invalid system ID, expected %q", metadata.GetSystemId())
	}

	// Step 3.1: recover the latest archived WAL file in the server
	latestStoredWAL, err := w.getLatestWALFileForCluster(req.GetClusterName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error while reading WAL directory")
	}

	// Step 3.2: recover the latest declared WAL gap
	var latestWALGapEnd string
	for _, gap := range metadata.GetGaps() {
		if gap.GetEnd() > latestWALGapEnd {
			latestWALGapEnd = gap.GetEnd()
		}
	}

	// Step 3.3: compute the server-side preferred WAL file, defaulting
	// it to the current client-side flush location if there is no information
	serverSideWALFile := max(latestStoredWAL, latestWALGapEnd)
	if serverSideWALFile == "" {
		serverSideWALFile = req.GetCurrentWalName()
	}

	w.logger.Info(
		"Server-side WAL start",
		"wal", serverSideWALFile,
		"latestWALGapEnd", latestWALGapEnd,
		"latestStoredWAL", latestStoredWAL)

	return &grpc.RequestWALStartResult{
		WalName: serverSideWALFile,
	}, nil
}

// ResetWALStream implements the GRPC call.
func (w *Implementation) ResetWALStream(
	ctx context.Context,
	req *grpc.ResetWALStreamRequest,
) (*grpc.ResetWALStreamResult, error) {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	// Step 1: recover cluster metadata
	var metadata *grpc.ClusterMetadata
	var err error

	if metadata, err = w.getClusterMetadata(ctx, req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.Internal, "error while reading cluster metadata: %v", err.Error())
	}

	// Step 2: ensure system ID is consistent
	if metadata.GetSystemId() != req.GetSystemId() {
		return nil, status.Errorf(codes.InvalidArgument, "invalid system ID, expected %q", metadata.GetSystemId())
	}

	// Step 3.1: recover the latest archived WAL file in the server
	latestStoredWAL, err := w.getLatestWALFileForCluster(req.GetClusterName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error while reading WAL directory")
	}

	if latestStoredWAL == "" {
		return nil, status.Errorf(codes.InvalidArgument, "empty WAL archive")
	}

	// Step 3.2: if the latest archived WAL is more recent than the forced
	// WAL start, bail out.
	if latestStoredWAL >= req.GetCurrentWalName() {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"proposed WAL starting point too old (latest WAL archived): %q",
			latestStoredWAL)
	}

	// Step 3.3: otherwise, we file a GAP report
	metadata.Gaps = append(metadata.Gaps, &grpc.WALGap{
		Ts:    timestamppb.Now(),
		Start: latestStoredWAL,
		End:   req.GetCurrentWalName(),
	})

	if err := w.writeClusterMetadata(ctx, req.GetClusterName(), metadata); err != nil {
		return nil, status.Errorf(codes.Internal, "error while writing cluster metadata: %v", err.Error())
	}

	return &grpc.ResetWALStreamResult{
		WalName: req.GetCurrentWalName(),
	}, nil
}

// getLatestWALFileForCluster gets the latest archived WAL for a certain cluster
//
//nolint:cyclop
func (w *Implementation) getLatestWALFileForCluster(
	clusterName string,
) (string, error) {
	clusterPath := path.Join(w.conn.BaseDir(), clusterName)
	readClusterDir, err := os.ReadDir(clusterPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		w.logger.Error(
			err,
			"while reading cluster directory",
			"clusterPath", clusterPath,
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

	latestWalDirectoryName = path.Join(clusterPath, latestWalDirectoryName)
	readWalDirectory, err := os.ReadDir(latestWalDirectoryName)
	if err != nil {
		w.logger.Error(err, "while reading directory", "latestWalDirectoryName", latestWalDirectoryName)
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

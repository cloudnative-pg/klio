package walserver

import (
	"context"
	"errors"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// GetMetadata implements the GetMetadata GRPC call.
func (w *Implementation) GetMetadata(
	ctx context.Context,
	req *grpc.GetMetadataRequest,
) (*grpc.ClusterMetadata, error) {
	if err := repository.ValidatePathComponent(req.GetClusterName()); err != nil {
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
	logger := log.FromContext(ctx)

	logger.Info(
		"RequestWALStart, negotiating the starting point with the client",
		"cluster", req.GetClusterName(),
		"systemID", req.GetSystemId(),
		"currentWAL", req.GetCurrentWalName())

	if err := repository.ValidatePathComponent(req.GetClusterName()); err != nil {
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
	latestStoredWAL, err := w.conn.GetLatestWALFileForCluster(ctx, req.GetClusterName())
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

	logger.Info(
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
	if err := repository.ValidatePathComponent(req.GetClusterName()); err != nil {
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
	latestStoredWAL, err := w.conn.GetLatestWALFileForCluster(ctx, req.GetClusterName())
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

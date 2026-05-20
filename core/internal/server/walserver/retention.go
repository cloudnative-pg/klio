package walserver

import (
	"context"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// SetFirstRequiredWAL drops all the WAL older than the passed one, effectively
// applying the retention policy. If there are WAL files pending transfer to
// tier2 in the queue, those will be preserved even if they are older than
// the requested first required WAL.
func (w *Implementation) SetFirstRequiredWAL(
	ctx context.Context,
	request *grpc.SetFirstRequiredWALRequest,
) (*grpc.SetFirstRequiredWALResult, error) {
	if w.isReadOnly {
		return nil, status.Error(codes.FailedPrecondition, errReadOnly.Error())
	}

	logger := log.FromContext(ctx)

	if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
		logger.Warning("Wrong cluster name used in WAL SetFirstRequired",
			"clusterName", request.GetClusterName())
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := repository.ValidateWalFileName(request.GetFirstRequiredWal()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", request.GetFirstRequiredWal())
	}

	firstRequiredWAL := request.GetFirstRequiredWal()

	// Clamp the first required WAL to the latest WAL that has been uploaded
	// to tier2 for this cluster. Anything newer than that may still be
	// pending transfer and must not be deleted from tier1.
	//
	// If no upload has ever been recorded for this cluster we cannot tell
	// which tier1 WALs have been transferred, so we refuse to delete
	// anything rather than risk losing pending WALs.
	if w.queue != nil {
		latestUploadedWAL, err := w.queue.GetLatestUploadedWAL(ctx, request.GetClusterName())
		if err != nil {
			logger.Error(err, "Error checking queue for latest uploaded WAL, proceeding with caution")
			// On error, we don't delete anything to be safe
			return &grpc.SetFirstRequiredWALResult{}, nil
		}

		if latestUploadedWAL == "" {
			logger.Info(
				"No latest uploaded WAL recorded for cluster, skipping WAL retention",
				"clusterName", request.GetClusterName(),
				"requestedFirstRequired", firstRequiredWAL,
			)

			return &grpc.SetFirstRequiredWALResult{}, nil
		}

		if strings.Compare(latestUploadedWAL, firstRequiredWAL) < 0 {
			logger.Info(
				"Adjusting first required WAL to preserve pending tier2 transfers",
				"requestedFirstRequired", firstRequiredWAL,
				"latestUploadedWAL", latestUploadedWAL,
			)
			firstRequiredWAL = latestUploadedWAL
		}
	}

	if err := w.conn.SetFirstRequiredOnCluster(
		ctx,
		request.GetClusterName(),
		firstRequiredWAL,
	); err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"while enforcing first required WAL %q for cluster %q: %v",
			firstRequiredWAL,
			request.GetClusterName(),
			err.Error(),
		)
	}

	return &grpc.SetFirstRequiredWALResult{}, nil
}

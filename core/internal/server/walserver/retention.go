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

	// Check if there are WAL files pending transfer to tier2 in the queue.
	// If so, we must not delete them even if they are older than the
	// requested first required WAL (based on tier1 backups).
	if w.queue != nil {
		oldestPendingWAL, err := w.queue.GetOldestPendingWAL(ctx, request.GetClusterName())
		if err != nil {
			logger.Error(err, "Error checking queue for pending WALs, proceeding with caution")
			// On error, we don't delete anything to be safe
			return &grpc.SetFirstRequiredWALResult{}, nil
		}

		if oldestPendingWAL != "" && strings.Compare(oldestPendingWAL, firstRequiredWAL) < 0 {
			logger.Info(
				"Adjusting first required WAL to preserve pending tier2 transfers",
				"requestedFirstRequired", firstRequiredWAL,
				"oldestPendingWAL", oldestPendingWAL,
			)
			firstRequiredWAL = oldestPendingWAL
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

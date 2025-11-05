package walserver

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// SetFirstRequiredWAL drops all the WAL older than the passed one, effectively
// applying the retention policy.
func (w *Implementation) SetFirstRequiredWAL(
	ctx context.Context,
	request *grpc.SetFirstRequiredWALRequest,
) (*grpc.SetFirstRequiredWALResult, error) {
	logger := log.FromContext(ctx)

	if err := repository.ValidatePathComponent(request.GetClusterName()); err != nil {
		logger.Warning("Wrong cluster name used in WAL SetFirstRequired",
			"clusterName", request.GetClusterName())
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := repository.ValidateWalFileName(request.GetFirstRequiredWal()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", request.GetFirstRequiredWal())
	}

	if err := w.conn.SetFirstRequiredOnCluster(
		ctx,
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

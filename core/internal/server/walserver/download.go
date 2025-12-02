package walserver

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// metadataFileName is the name of the file containing the
// cluster metadata.
const metadataFileName = "metadata"

// Get implements the relative GRPC call.
func (w *Implementation) Get(req *grpc.GetRequest, res grpc.WAL_GetServer) error { //nolint:cyclop
	logger := log.FromContext(res.Context())
	if err := repository.ValidatePathComponent(req.GetClusterName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := repository.ValidatePathComponent(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

	if err := repository.ValidateWalFileName(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", req.GetWalName())
	}

	ctx, span := tracer.Start(
		res.Context(),
		opentelemetry.GetWalSpan,
		trace.WithAttributes(
			attribute.String("cluster_name", req.GetClusterName()),
			attribute.String("wal_name", req.GetWalName()),
		),
	)
	defer span.End()

	walReader, err := repository.NewReader(w.conn, req.GetClusterName(), req.GetWalName(), tracer)
	if errors.Is(err, os.ErrNotExist) {
		span.RecordError(fmt.Errorf("WAL not found: %v/%v", req.GetClusterName(), req.GetWalName()))
		return status.Errorf(codes.NotFound, "WAL not found: %v/%v", req.GetClusterName(), req.GetWalName())
	}
	if err != nil {
		return status.Errorf(codes.Internal, "error while reading WAL (opening): %v", err.Error())
	}

	for {
		readCtx, readSpan := tracer.Start(ctx, opentelemetry.ReadBlockSpan)
		readBytes, readError := walReader.ReadBlock(readCtx)
		readSpan.SetAttributes(
			attribute.Int("num_bytesread", len(readBytes)))
		readSpan.End()
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading into buffer): %v", readError.Error())
		}

		_, writeSpan := tracer.Start(ctx, opentelemetry.SendBlockSpan)
		err := res.Send(&grpc.GetResult{WalBlock: readBytes, SegmentSize: walReader.GetFileLength()})
		writeSpan.SetAttributes(
			attribute.Int("num_bytesread", len(readBytes)))
		writeSpan.End()
		if err != nil {
			return status.Errorf(codes.Internal, "error while writing WAL block (sending to client GRPC): %v",
				err.Error())
		}

		if errors.Is(readError, io.EOF) {
			logger.Debug("WAL read completed", "name", req.GetWalName(), "cluster", req.GetClusterName())
			break
		}
	}

	return nil
}

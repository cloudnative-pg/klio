package walserver

import (
	"errors"
	"io"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
)

// metadataFileName is the name of the file containing the
// cluster metadata.
const metadataFileName = "metadata"

// Get implements the relative GRPC call.
func (w *Implementation) Get(req *grpc.GetRequest, res grpc.WAL_GetServer) error { //nolint:cyclop
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := validatePathComponent(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

	if err := validateWalFileName(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", req.GetWalName())
	}

	ctx, span := tracer.Start(
		res.Context(),
		"klio.wal.get",
		trace.WithAttributes(
			attribute.String("cluster_name", req.GetClusterName()),
			attribute.String("wal_name", req.GetWalName()),
		),
	)
	defer span.End()

	walReader, err := NewReader(w.conn, req.GetClusterName(), req.GetWalName())
	if errors.Is(err, os.ErrNotExist) {
		return status.Errorf(codes.NotFound, "WAL not found: %v/%v", req.GetClusterName(), req.GetWalName())
	}
	if err != nil {
		return status.Errorf(codes.Internal, "error while reading WAL (opening): %v", err.Error())
	}

	for {
		_, readSpan := tracer.Start(ctx, "klio.wal.get.read")
		readBytes, readError := walReader.ReadBlock()
		readSpan.SetAttributes(
			attribute.Int("num_bytesread", len(readBytes)))
		readSpan.End()
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading into buffer): %v", readError.Error())
		}

		_, writeSpan := tracer.Start(ctx, "klio.wal.get.write")
		err := res.Send(&grpc.GetResult{WalBlock: readBytes, SegmentSize: walReader.GetFileLength()})
		writeSpan.SetAttributes(
			attribute.Int("num_bytesread", len(readBytes)))
		writeSpan.End()
		if err != nil {
			return status.Errorf(codes.Internal, "error while reading WAL block (sending to client GRPC): %v", err.Error())
		}

		if errors.Is(readError, io.EOF) {
			w.logger.Debug("WAL read completed", "name", req.GetWalName(), "cluster", req.GetClusterName())
			break
		}
	}

	return nil
}

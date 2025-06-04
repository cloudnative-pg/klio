package walserver

import (
	"errors"
	"io"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/EnterpriseDB/klio/internal/grpc"
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

	walReader, err := NewReader(w.conn, req.GetClusterName(), req.GetWalName())
	if errors.Is(err, os.ErrNotExist) {
		return status.Errorf(codes.NotFound, "WAL not found: %v/%v", req.GetClusterName(), req.GetWalName())
	}
	if err != nil {
		return status.Errorf(codes.Internal, "error while reading WAL (opening): %v", err.Error())
	}

	for {
		readBytes, readError := walReader.ReadBlock()
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading into buffer): %v", readError.Error())
		}

		if err := res.Send(&grpc.GetResult{WalBlock: readBytes, SegmentSize: walReader.GetFileLength()}); err != nil {
			return status.Errorf(codes.Internal, "error while reading WAL block (sending to client GRPC): %v", err.Error())
		}

		if errors.Is(readError, io.EOF) {
			w.logger.Debug("WAL read completed", "name", req.GetWalName(), "cluster", req.GetClusterName())
			break
		}
	}

	return nil
}

package walserver

import (
	"errors"
	"io"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/EnterpriseDB/klio/internal/grpc"
)

// GetWAL implements the relative GRPC call.
func (w *Implementation) GetWAL(req *grpc.GetWALRequest, res grpc.WAL_GetWALServer) error {
	walReader, err := NewWALReader(w.conn, req.GetClusterName(), req.GetWalName())
	if os.IsNotExist(err) {
		return status.Errorf(codes.NotFound, "WAL not found: %v/%v", req.GetClusterName(), req.GetWalName())
	}
	if err != nil {
		return status.Errorf(codes.Internal, "error while reading WAL (opening): %v", err.Error())
	}

	buffer := make([]byte, 4096)
	for {
		readBytes, readError := walReader.Read(buffer)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading into buffer): %v", readError.Error())
		}

		if err := res.Send(&grpc.GetWALResult{WalBlock: buffer[:readBytes]}); err != nil {
			return status.Errorf(codes.Internal, "error while reading WAL block (sending to client GRPC): %v", err.Error())
		}

		if errors.Is(readError, io.EOF) {
			w.logger.Debug("WAL read completed", "name", req.GetWalName(), "cluster", req.GetClusterName())
			break
		}
	}

	return nil
}

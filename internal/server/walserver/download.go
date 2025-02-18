package walserver

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/EnterpriseDB/klio/internal/grpc"
)

// GetWAL implements the relative GRPC call.
func (w *Implementation) GetWAL(req *grpc.GetWALRequest, res grpc.WAL_GetWALServer) error {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := validatePathComponent(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

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

// GetLatestWAL implements the relative GRPC call.
//
//nolint:cyclop
func (w *Implementation) GetLatestWAL(
	_ context.Context,
	req *grpc.GetLatestWALRequest,
) (*grpc.GetLatestWALResult, error) {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	clusterPath := path.Join(w.conn.BaseDir(), req.GetClusterName())
	readClusterDir, err := os.ReadDir(clusterPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &grpc.GetLatestWALResult{
				WalName: nil,
			}, nil
		}

		w.logger.Error(
			"while reading cluster directory",
			"clusterPath", clusterPath,
			"err", err,
		)

		return nil, status.Errorf(codes.Internal, "error while reading directory")
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
		return &grpc.GetLatestWALResult{
			WalName: nil,
		}, nil
	}

	latestWalDirectoryName = path.Join(clusterPath, latestWalDirectoryName)
	readWalDirectory, err := os.ReadDir(latestWalDirectoryName)
	if err != nil {
		w.logger.Error("while reading directory", "latestWalDirectoryName", latestWalDirectoryName, "err", err)
		return nil, status.Errorf(codes.Internal, "error while reading directory")
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
		return &grpc.GetLatestWALResult{
			WalName: nil,
		}, nil
	}

	return &grpc.GetLatestWALResult{
		WalName: &lastWal,
	}, nil
}

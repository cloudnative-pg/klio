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

// Get implements the relative GRPC call.
func (w *Implementation) Get(req *grpc.GetRequest, res grpc.WAL_GetServer) error {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := validatePathComponent(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

	walReader, err := NewWALReader(w.conn, req.GetClusterName(), req.GetWalName())
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

// GetLatest implements the relative GRPC call.
//
//nolint:cyclop
func (w *Implementation) GetLatest(
	_ context.Context,
	req *grpc.GetLatestRequest,
) (*grpc.GetLatestResult, error) {
	if err := validatePathComponent(req.GetClusterName()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	clusterPath := path.Join(w.conn.BaseDir(), req.GetClusterName())
	readClusterDir, err := os.ReadDir(clusterPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &grpc.GetLatestResult{
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
		return &grpc.GetLatestResult{
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
		return &grpc.GetLatestResult{
			WalName: nil,
		}, nil
	}

	return &grpc.GetLatestResult{
		WalName: &lastWal,
	}, nil
}

package klioserver

import (
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/EnterpriseDB/klio/internal/klioserver/grpc"
)

var (
	errEmptyClusterName = errors.New("empty cluster name")
	errEmptyWALName     = errors.New("empty WAL name")
	errEmptySegmentSize = errors.New("empty segment size")
)

type incoherentRequestError struct {
	expectedValue string
	foundValue    string
	involvedField string
}

func (e *incoherentRequestError) Error() string {
	return fmt.Sprintf(
		"incoherent %s, expected %s found %s",
		e.involvedField,
		e.expectedValue,
		e.foundValue,
	)
}

type walUploadBlockMetadata struct {
	clusterName string
	walFileName string
	segmentSize uint64
}

func (m *walUploadBlockMetadata) handleRequest(request *grpc.UploadWALRequest) error {
	if request.GetClusterName() == "" {
		return errEmptyClusterName
	}
	if request.GetWalName() == "" {
		return errEmptyWALName
	}
	if request.GetSegmentSize() == 0 {
		return errEmptySegmentSize
	}

	if m.clusterName == "" {
		m.clusterName = request.GetClusterName()
	} else if m.clusterName != request.GetClusterName() {
		return &incoherentRequestError{
			involvedField: "cluster name",
			expectedValue: m.clusterName,
			foundValue:    request.GetClusterName(),
		}
	}

	if m.walFileName == "" {
		m.walFileName = request.GetWalName()
	} else if m.walFileName != request.GetWalName() {
		return &incoherentRequestError{
			involvedField: "wal name",
			expectedValue: m.walFileName,
			foundValue:    request.GetWalName(),
		}
	}

	if m.segmentSize == 0 {
		m.segmentSize = request.GetSegmentSize()
	} else if m.segmentSize != request.GetSegmentSize() {
		return &incoherentRequestError{
			involvedField: "wal segment size",
			expectedValue: fmt.Sprintf("%v", m.segmentSize),
			foundValue:    fmt.Sprintf("%v", request.GetSegmentSize()),
		}
	}

	return nil
}

// UploadWAL uploads a new WAL to the data store.
//
//nolint:cyclop
func (w *WALServerImplementation) UploadWAL(req grpc.WAL_UploadWALServer) error {
	var blockMeta walUploadBlockMetadata
	var walBuffer *WALWriter
	var writtenSize uint64

	for {
		request, err := req.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "error while reading WAL block: %v", err.Error())
		}

		if err := blockMeta.handleRequest(request); err != nil {
			return status.Errorf(codes.InvalidArgument, "%s", err.Error())
		}

		if walBuffer == nil {
			walBuffer, err = NewWALWriter(w.cfg.WALPath, blockMeta.clusterName, blockMeta.walFileName)
			if err != nil {
				return status.Errorf(codes.Internal, "error while opening new WAL: %v", err.Error())
			}
		}

		bytesWritten, err := walBuffer.Write(request.GetWalBlock())
		if err != nil {
			return status.Errorf(codes.Internal, "error while writing WAL: %v", err.Error())
		}

		if err = walBuffer.Flush(); err != nil {
			return status.Errorf(codes.Internal, "error while flushing WAL: %v", err.Error())
		}

		//nolint:gosec
		writtenSize += uint64(bytesWritten)
	}

	if walBuffer == nil {
		if err := req.SendAndClose(&grpc.UploadWALResult{
			WrittenSize: 0,
		}); err != nil {
			return status.Errorf(codes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}
		return nil
	}

	if writtenSize != blockMeta.segmentSize || writtenSize == 0 {
		if err := walBuffer.Close(); err != nil {
			return status.Errorf(codes.Internal, "error while closing (partial) WAL: %v", err.Error())
		}
	} else {
		if err := walBuffer.CloseMarkDone(); err != nil {
			return status.Errorf(codes.Internal, "error while closing (done) WAL: %v", err.Error())
		}
	}

	if err := req.SendAndClose(&grpc.UploadWALResult{
		WrittenSize: writtenSize,
	}); err != nil {
		return status.Errorf(codes.Internal, "error while sending response: %v", err.Error())
	}

	return nil
}

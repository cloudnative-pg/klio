package grpcclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	klioGRPC "github.com/EnterpriseDB/klio/internal/grpc"
)

// StoreWAL uploads a WAL in the WAL server
// Important: this function uploads a full WAL file.
func (c *Connection) StoreWAL(ctx context.Context, name string, content []byte) error {
	stream, err := c.walClient.Put(ctx)
	if err != nil {
		return fmt.Errorf("while starting uploading a WAL file: %w", err)
	}

	walReader := bytes.NewBuffer(content)

	buffer := make([]byte, 4096)
	for {
		readBytes, readError := walReader.Read(buffer)
		if readError != nil && !errors.Is(readError, io.EOF) {
			return fmt.Errorf("error while reading WAL (reading from buffer): %w", readError)
		}

		if err := stream.Send(&klioGRPC.PutRequest{
			ClusterName: c.cfg.ClusterName,
			WalName:     name,
			SegmentSize: uint64(len(content)),
			WalBlock:    buffer[:readBytes],
		}); err != nil {
			return fmt.Errorf("error while sending WAL block (sending via GRPC): %w", err)
		}

		if errors.Is(readError, io.EOF) {
			break
		}
	}

	result, err := stream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("while flushing WAL file: %w", err)
	}

	if result.GetWrittenSize() != uint64(len(content)) {
		return &IncompleteWALFileError{
			uploadedSize: result.GetWrittenSize(),
			expectedSize: uint64(len(content)),
		}
	}

	return nil
}

// StoreHistoryFile uses the underlying GRPC connection to store a history file.
func (c *Connection) StoreHistoryFile(ctx context.Context, name string, content []byte) error {
	return c.StoreWAL(ctx, name, content)
}

// GetLatestWALFile gets the latest WAL file from the repository.
func (c *Connection) GetLatestWALFile(ctx context.Context) (string, error) {
	result, err := c.walClient.GetLatest(ctx, &klioGRPC.GetLatestRequest{
		ClusterName: c.cfg.ClusterName,
	})
	if err != nil {
		return "", fmt.Errorf("while querying for the latest WAL file: %w", err)
	}

	return result.GetWalName(), nil
}

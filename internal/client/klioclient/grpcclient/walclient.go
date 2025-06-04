package grpcclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/common"
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

// RequestWALStart requests the WAL streaming starting point.
func (c *Connection) RequestWALStart(ctx context.Context, opts common.WALStartOptions) (string, error) {
	response, err := c.walClient.RequestWALStart(
		ctx,
		&klioGRPC.RequestWALStartRequest{
			ClusterName:    opts.ClusterName,
			SystemId:       opts.SystemID,
			CurrentWalName: opts.ClientPreferredWALName,
		},
	)
	if err != nil {
		return "", fmt.Errorf("while negotiating replication starting point: %w", err)
	}

	return response.GetWalName(), nil
}

// ResetWALStream reset the server-side replication status to the passed starting point.
func (c *Connection) ResetWALStream(ctx context.Context, opts common.WALStartOptions) (string, error) {
	response, err := c.walClient.RequestWALStart(
		ctx,
		&klioGRPC.RequestWALStartRequest{
			ClusterName:    opts.ClusterName,
			SystemId:       opts.SystemID,
			CurrentWalName: opts.ClientPreferredWALName,
		},
	)
	if err != nil {
		return "", fmt.Errorf("while resetting replication starting point: %w", err)
	}

	return response.GetWalName(), nil
}

// GetMetadata reset the server-side replication status to the passed starting point.
func (c *Connection) GetMetadata(ctx context.Context) (*klioGRPC.ClusterMetadata, error) {
	response, err := c.walClient.GetMetadata(ctx, &klioGRPC.GetMetadataRequest{
		ClusterName: c.cfg.ClusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("while getting cluster Klio metadata: %w", err)
	}

	return response, nil
}

package common

import (
	"context"

	"github.com/EnterpriseDB/klio/pkg/klioclient/types"
)

// Client is the interface that wraps the backend storage.
type Client interface {
	// StoreWAL upload a WAL file to a remote store
	StoreWAL(ctx context.Context, name string, content []byte) error

	// StoreWALStreaming streams a WAL file to a remote store
	StoreWALStreaming(ctx context.Context, name string, segmentSize uint64) (WALStream, error)

	// GetWAL recovers a WAL file from a remote store
	GetWAL(ctx context.Context, walName string) (*types.WalEntry, error)

	// Close closes the connection
	Close(ctx context.Context) error
}

// WALStream streams a WAL file to a remote store, block by block.
type WALStream interface {
	// SendBlock sends a WAL Block
	SendBlock(ctx context.Context, block []byte) error

	// Close closes the WAL streaming session
	Close(ctx context.Context) error
}

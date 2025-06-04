package common

import (
	"context"
	"io"

	"github.com/EnterpriseDB/klio/internal/grpc"
)

// WALClientStreamer is implemented by clients supporting WAL streaming
// and are generic WAL clients too.
type WALClientStreamer interface {
	WALClient
	WALStreamer
}

// WALStreamer is implemented by clients that support streaming WALs
// to a remote location block by block.
type WALStreamer interface {
	// StoreWALStreaming streams a WAL file to a remote store
	StoreWALStreaming(ctx context.Context, name string, segmentSize uint64) (*WALUploader, error)

	// GetWALStreaming recovers a WAL file from a remote store
	GetWALStreaming(ctx context.Context, walName string, o io.Writer) error
}

// WALClient is the interface that wraps the backend WAL storage.
type WALClient interface {
	// StoreWAL upload a WAL file to a remote store
	StoreWAL(ctx context.Context, name string, content []byte) error

	// StoreHistoryFile upload an history file to a remote store
	StoreHistoryFile(ctx context.Context, name string, content []byte) error

	// RequestWALStart requests the WAL streaming starting point
	RequestWALStart(ctx context.Context, opts WALStartOptions) (string, error)

	// RequestWALStart requests the WAL streaming starting point
	ResetWALStream(ctx context.Context, opts WALStartOptions) (string, error)

	// GetMetadata gets the metadata of the configured cluster
	GetMetadata(ctx context.Context) (*grpc.ClusterMetadata, error)

	// Close closes the connection
	Close(ctx context.Context) error
}

// WALStartOptions are the options transferred by the client
// to the WAL server before starting the replication.
type WALStartOptions struct {
	// ClusterName is the name of the cluster
	ClusterName string

	// SystemID is the detected system ID
	SystemID string

	// ClientPreferredWALName is the WAL name from which the client
	// would prefer to start streaming
	ClientPreferredWALName string
}

// WALUploaderImpl is the underlying implementation of a WAL
// uploader.
type WALUploaderImpl interface {
	// SendBlock sends a WAL Block
	SendBlock(ctx context.Context, block []byte) error

	// Close closes the WAL streaming session
	Close(ctx context.Context) error
}

// WALUploader allows the user the upload a WAL file to a remote store, block by block.
type WALUploader struct {
	impl WALUploaderImpl
}

// NewWALUploader creates a WAL uploader given the underlying implementation.
func NewWALUploader(impl WALUploaderImpl) *WALUploader {
	return &WALUploader{
		impl: impl,
	}
}

// SendBlock sends a WAL Block.
func (u *WALUploader) SendBlock(ctx context.Context, block []byte) error {
	return u.impl.SendBlock(ctx, block) //nolint:wrapcheck
}

// Close closes the WAL streaming session.
func (u *WALUploader) Close(ctx context.Context) error {
	return u.impl.Close(ctx) //nolint:wrapcheck
}

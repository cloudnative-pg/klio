package common

import (
	"context"

	"github.com/EnterpriseDB/klio/internal/client/klioclient/types"
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
}

// WALClient is the interface that wraps the backend WAL storage.
type WALClient interface {
	// StoreWAL upload a WAL file to a remote store
	StoreWAL(ctx context.Context, name string, content []byte) error

	// StoreHistoryFile upload an history file to a remote store
	StoreHistoryFile(ctx context.Context, name string, content []byte) error

	// GetWAL recovers a WAL file from a remote store
	GetWAL(ctx context.Context, walName string) (*types.Entry, error)

	// Close closes the connection
	Close(ctx context.Context) error
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

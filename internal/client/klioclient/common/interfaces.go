package common

import (
	"context"
	"fmt"
	"io"
)

// WALClientStreamer is implemented by clients supporting WAL streaming
// and are generic WAL clients too.
type WALClientStreamer interface {
	WALClient
	WALStreamer
}

// ErrMissingWALFile is raised when the client requires a WAL file
// that doesn't exist on the server
var ErrMissingWALFile = fmt.Errorf("non existing WAL file")

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

	// GetLatestWALFile gets the latest WAL file that have been archived for this server
	GetLatestWALFile(ctx context.Context) (string, error)

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

package common

import (
	"context"
)

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

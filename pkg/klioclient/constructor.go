package klioclient

import (
	"context"
	"log/slog"

	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/grpcclient"
	"github.com/EnterpriseDB/klio/pkg/klioclient/kopia"
	"github.com/EnterpriseDB/klio/pkg/klioclient/types"
)

// Client is the interface that wraps the backend storage.
type Client interface {
	// StoreWAL upload a WAL file to a remote store
	StoreWAL(ctx context.Context, name string, content []byte) error

	// GetWAL recovers a WAL file from a remote store
	GetWAL(ctx context.Context, walName string) (*types.WalEntry, error)

	// Close closes the connection
	Close(ctx context.Context) error
}

// NewKopiaClient creates a client to interact with Kopia.
func NewKopiaClient(
	ctx context.Context,
	logger *slog.Logger,
	serverConfig *config.KopiaRepositoryClientConfig,
) (*kopia.Connection, error) {
	//nolint: wrapcheck
	return kopia.Connect(ctx, logger, serverConfig)
}

// NewKlioClient creates a client to interact with Kopia.
func NewKlioClient(
	logger *slog.Logger,
	klioConfig *config.KlioRepositoryClientConfig,
) (*grpcclient.Connection, error) {
	//nolint: wrapcheck
	return grpcclient.Connect(logger, klioConfig)
}

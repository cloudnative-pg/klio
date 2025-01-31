package klioclient

import (
	"context"
	"log/slog"

	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/EnterpriseDB/klio/pkg/klioclient/kopia"
)

// Client is the interface that wraps the backend storage.
type Client interface {
	StoreWAL(ctx context.Context, name string, content []byte) error
}

// NewKopiaClient creates a client to interact with Kopia.
func NewKopiaClient(
	ctx context.Context,
	logger *slog.Logger,
	serverConfig *config.Server,
) (Client, error) {
	//nolint: wrapcheck
	return kopia.Connect(ctx, logger, serverConfig)
}

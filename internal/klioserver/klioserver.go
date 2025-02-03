package klioserver

import (
	"log/slog"

	"github.com/EnterpriseDB/klio/internal/klioserver/grpc"
	"github.com/EnterpriseDB/klio/pkg/config"
)

// WALServerImplementation is the implementation of the WAL server.
type WALServerImplementation struct {
	grpc.UnimplementedWALServer

	cfg    *config.KlioServerConfig
	logger *slog.Logger
}

// NewWALServerImplementation creates a new WAL server implementation.
func NewWALServerImplementation(
	logger *slog.Logger,
	cfg *config.KlioServerConfig,
) *WALServerImplementation {
	return &WALServerImplementation{
		cfg:    cfg,
		logger: logger,
	}
}

package klioserver

import (
	"log/slog"

	"github.com/EnterpriseDB/klio/internal/klioserver/grpc"
	"github.com/EnterpriseDB/klio/internal/klioserver/repository"
)

// WALServerImplementation is the implementation of the WAL server.
type WALServerImplementation struct {
	grpc.UnimplementedWALServer

	logger *slog.Logger
	conn   *repository.Connection
}

// NewWALServerImplementation creates a new WAL server implementation.
func NewWALServerImplementation(
	logger *slog.Logger,
	conn *repository.Connection,
) *WALServerImplementation {
	return &WALServerImplementation{
		logger: logger,
		conn:   conn,
	}
}

package walserver

import (
	"log/slog"

	"github.com/EnterpriseDB/klio/internal/grpc"
	"github.com/EnterpriseDB/klio/internal/server/walserver/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	logger *slog.Logger
	conn   *repository.Connection
}

// New creates a new WAL server implementation.
func New(
	logger *slog.Logger,
	conn *repository.Connection,
) *Implementation {
	return &Implementation{
		logger: logger,
		conn:   conn,
	}
}

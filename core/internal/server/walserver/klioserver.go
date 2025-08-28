package walserver

import (
	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/server/walserver/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	// TODO(leonardoce): pls remove me
	logger log.Logger
	conn   *repository.Connection
}

// New creates a new WAL server implementation.
func New(
	logger log.Logger,
	conn *repository.Connection,
) *Implementation {
	return &Implementation{
		logger: logger,
		conn:   conn,
	}
}

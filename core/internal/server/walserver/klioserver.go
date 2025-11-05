package walserver

import (
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	conn    *repository.Connection
	metrics *repository.Metrics
}

// New creates a new WAL server implementation.
func New(
	conn *repository.Connection,
) *Implementation {
	return &Implementation{
		conn:    conn,
		metrics: NewMetrics(),
	}
}

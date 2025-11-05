package walserver

import (
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	conn       *repository.Connection
	metrics    *repository.Metrics
	isReadOnly bool
}

// Options is the structure describing the behavior of
// a WAL server.
type Options struct {
	Connection *repository.Connection
	ReadOnly   bool
}

// New creates a new WAL server implementation.
func New(
	opts Options,
) *Implementation {
	return &Implementation{
		conn:       opts.Connection,
		isReadOnly: opts.ReadOnly,
		metrics:    NewMetrics(),
	}
}

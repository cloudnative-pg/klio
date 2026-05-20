package walserver

import (
	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/queue"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// Implementation is the implementation of the WAL server.
type Implementation struct {
	grpc.UnimplementedWALServer

	conn       *repository.Connection
	metrics    *repository.Metrics
	isReadOnly bool
	queue      *queue.Conn
}

// Options is the structure describing the behavior of
// a WAL server.
type Options struct {
	// Connection is the repository to be used with this WAL server
	Connection *repository.Connection

	// ReadOnly is true when the repository should deny any write attempt
	ReadOnly bool

	// Queue is the connection to the Queue
	Queue *queue.Conn
}

// New creates a new WAL server implementation.
func New(
	opts Options,
) *Implementation {
	return &Implementation{
		conn:       opts.Connection,
		isReadOnly: opts.ReadOnly,
		metrics: &repository.Metrics{
			WalWrittenBytes:       opentelemetry.WalServer.WalWrittenBytes,
			WalWritten:            opentelemetry.WalServer.WalWritten,
			LatestWrittenTime:     opentelemetry.WalServer.LatestWrittenTime,
			LatestWrittenLSN:      opentelemetry.WalServer.LatestWrittenLSN,
			LatestWrittenTimeline: opentelemetry.WalServer.LatestWrittenTimeline,
		},
		queue: opts.Queue,
	}
}

package infrastructure

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/EnterpriseDB/klio/pkg/config"
)

// Service is the infrastructure service
type Service interface {
	Postgres
}

// Postgres details the infrastructure Postgres capabilities
type Postgres interface {
	// GetWalSegmentSize returns the size of the WAL segment
	GetWalSegmentSize(ctx context.Context) (uint64, error)

	// NewConn returns the connection to the database
	NewConn(ctx context.Context) (*pgconn.PgConn, error)
}

type impl struct {
	config *config.Data
	logger *slog.Logger
}

// New creates a new infrastructure service
func New(cfg *config.Data, log *slog.Logger) Service {
	return &impl{
		config: cfg,
		logger: log.With("service", "infrastructure"),
	}
}

func (s *impl) NewConn(ctx context.Context) (*pgconn.PgConn, error) {
	return pgconn.Connect(ctx, s.config.Source.DSN)
}

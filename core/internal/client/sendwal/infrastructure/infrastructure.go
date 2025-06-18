package infrastructure

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Postgres details the infrastructure Postgres capabilities.
type Postgres struct {
	config *config.Data
	logger *slog.Logger
}

// NewPostgres creates a new PostgreSQL infrastructure.
func NewPostgres(cfg *config.Data, log *slog.Logger) *Postgres {
	return &Postgres{
		config: cfg,
		logger: log.With("service", "infrastructure"),
	}
}

// NewConn returns the connection to the database.
func (s *Postgres) NewConn(ctx context.Context) (*pgconn.PgConn, error) {
	//nolint:wrapcheck
	return pgconn.Connect(ctx, s.config.Source.DSN)
}

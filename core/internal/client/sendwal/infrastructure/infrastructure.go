package infrastructure

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Postgres details the infrastructure Postgres capabilities.
type Postgres struct {
	config *config.Data
	logger log.Logger
}

// NewPostgres creates a new PostgreSQL infrastructure.
func NewPostgres(cfg *config.Data, log log.Logger) *Postgres {
	return &Postgres{
		config: cfg,
		logger: log.WithValues("service", "infrastructure"),
	}
}

// NewConn returns the connection to the database.
func (s *Postgres) NewConn(ctx context.Context) (*pgconn.PgConn, error) {
	//nolint:wrapcheck
	return pgconn.Connect(ctx, s.config.Source.DSN)
}

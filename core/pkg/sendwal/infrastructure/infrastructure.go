/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package infrastructure

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres details the infrastructure Postgres capabilities.
type Postgres struct {
	dsn    string
	logger log.Logger
}

// NewPostgres creates a new PostgreSQL infrastructure. The passed dsn is
// used, as-is, to open connections to the source PostgreSQL instance.
func NewPostgres(dsn string, log log.Logger) *Postgres {
	return &Postgres{
		dsn:    dsn,
		logger: log.WithValues("service", "infrastructure"),
	}
}

// NewConn returns the connection to the database.
func (s *Postgres) NewConn(ctx context.Context) (*pgconn.PgConn, error) {
	//nolint:wrapcheck
	return pgconn.Connect(ctx, s.dsn)
}

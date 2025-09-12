package walserver

import (
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("klio.wal_server") //nolint:gochecknoglobals

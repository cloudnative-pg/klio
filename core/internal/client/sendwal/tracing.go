package sendwal

import (
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("klio.wal_client") //nolint:gochecknoglobals

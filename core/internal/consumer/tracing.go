package consumer

import (
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("klio.consumer") //nolint:gochecknoglobals

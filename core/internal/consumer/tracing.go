package consumer

import (
	"go.opentelemetry.io/otel"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

var tracer = otel.Tracer(opentelemetry.TracerConsumer) //nolint:gochecknoglobals

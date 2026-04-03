package walserver

import "github.com/cloudnative-pg/klio/core/internal/opentelemetry"

//nolint:gochecknoinits
func init() {
	opentelemetry.InitWalServerMetrics()
}

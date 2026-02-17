package kopiaserver

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// Config contains the information required to start up a Kopia server.
type Config struct {
	// ListenAddress is the <host>:<port> specification that is
	// used to create the listening socket.
	ListenAddress string

	// Kopia server control username. This credential can be used
	// to access the REST control API.
	ServerControlUser string

	// Kopia server control password. This credential can be used
	// to access the REST control API.
	ServerControlPassword string
}

// start runs a Kopia server with the passed configuration.
func start(ctx context.Context, configFile string, cfg *Config, tls *config.TLSConfig) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	// Create observable uptime metric
	serverStartTime := time.Now()
	_, err = otel.Meter(opentelemetry.Meter).Float64ObservableGauge(
		kopiaServerUptimeMetricName,
		metric.WithDescription("Kopia server uptime in seconds"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(time.Since(serverStartTime).Seconds())
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("while creating uptime metric: %w", err)
	}

	// Start the snapshot metrics collector
	metricsCollector, err := newSnapshotMetricsCollector(
		configFile,
		time.Minute,
		contextLogger)
	if err != nil {
		return fmt.Errorf("while creating snapshot metrics collector: %w", err)
	}

	go metricsCollector.Start(ctx)
	defer metricsCollector.Stop()

	wrapper := kopia.Client{
		KopiaBinary: kopiaBinary,
		ConfigFile:  configFile,
	}

	if err := wrapper.RunServer(ctx, kopia.ServerOptions{
		TLSCert:               tls.TLSCert,
		TLSKey:                tls.TLSKey,
		ClientCACertFile:      tls.ClientCACertFile,
		ListenAddress:         cfg.ListenAddress,
		ServerControlUser:     cfg.ServerControlUser,
		ServerControlPassword: cfg.ServerControlPassword,
	}); err != nil {
		return fmt.Errorf("while starting the kopia server: %w", err)
	}

	return nil
}

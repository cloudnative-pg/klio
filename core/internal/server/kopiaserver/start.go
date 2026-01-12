package kopiaserver

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// kopiaCommand is the name of the kopia binary.
const kopiaCommand = "kopia"

// Config contains the information required to start up a Kopia server.
type Config struct {
	// EncryptionPassword is the repository encryption password
	EncryptionPassword string

	// CacheDirectory is used by the Kopia server to cache blobs.
	// This is important for remote repositories.
	CacheDirectory string

	// ListenAddress is the <host>:<port> specification that is
	// used to create the listening socket.
	ListenAddress string

	// ReadOnly is true when the server should deny write access,
	// avoiding accidental changes.
	ReadOnly bool
}

// start runs a Kopia server with the passed configuration.
func start(ctx context.Context, configFile string, cfg *Config, tls *config.TLSConfig) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	// Enable ACLs
	enableACLs(ctx, kopiaBinary, configFile, cfg.EncryptionPassword)

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
		cfg.EncryptionPassword,
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
		TLSCert:          tls.TLSCert,
		TLSKey:           tls.TLSKey,
		ClientCACertFile: tls.ClientCACertFile,
		ListenAddress:    cfg.ListenAddress,
		ReadOnly:         cfg.ReadOnly,
	}); err != nil {
		return fmt.Errorf("while starting the kopia server: %w", err)
	}

	return nil
}

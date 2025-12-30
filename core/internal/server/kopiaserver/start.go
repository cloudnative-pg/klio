package kopiaserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

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

	// Start the Kopia server
	args := []string{
		"server", "start",
		"--tls-key-file=" + tls.TLSKey,
		"--tls-cert-file=" + tls.TLSCert,
		"--tls-ca-file=" + tls.ClientCACertFile,
		"--config-file=" + configFile,
		"--cache-directory=" + cfg.CacheDirectory,
		"--address=" + cfg.ListenAddress,
		"--disable-file-logging",
		"--json-log-console",
	}

	kopiaServer := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaServer.Stdout = os.Stdout
	kopiaServer.Stderr = os.Stderr
	contextLogger.Info("Starting Kopia server", "args", kopiaServer.Args)

	if err := kopiaServer.Start(); err != nil {
		return fmt.Errorf("while starting the kopia server: %w", err)
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
		cfg.EncryptionPassword,
		time.Minute,
		contextLogger)
	if err != nil {
		return fmt.Errorf("while creating snapshot metrics collector: %w", err)
	}

	go metricsCollector.Start(ctx)
	defer metricsCollector.Stop()

	if err := kopiaServer.Wait(); err != nil {
		return fmt.Errorf("while running the kopia server: %w", err)
	}

	return nil
}

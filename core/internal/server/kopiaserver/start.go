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

// start runs a Kopia server with the passed configuration.
func start(ctx context.Context, configFile string, cfg *config.BaseServerConfig) error {
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
		"--tls-key-file=" + cfg.TLSKey,
		"--tls-cert-file=" + cfg.TLSCert,
		"--tls-ca-file=" + cfg.ClientCACertFile,
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

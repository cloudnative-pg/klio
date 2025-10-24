package kopiaserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/kopia/kopia/repo/content"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// kopiaCommand is the name of the kopia binary.
const kopiaCommand = "kopia"

// Start runs a Kopia server with the passed configuration.
//
//nolint:cyclop
func Start(ctx context.Context, cfg *config.BaseServerConfig) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	storage, err := filesystem.New(ctx, &filesystem.Options{Path: cfg.RepositoryDirectory}, true)
	if err != nil {
		return fmt.Errorf("while creating Kopia filesystem storage: %w", err)
	}

	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	defer func() {
		if err := os.Remove(configFile.Name()); err != nil {
			contextLogger.Warning(
				"Error while removing temporary configuration file",
				"err", err,
				"configFile", configFile.Name(),
			)
		}
	}()

	if err := repo.Connect(ctx, configFile.Name(), storage, cfg.EncryptionPassword, &repo.ConnectOptions{
		// These are just the default values... should
		// we set something else?
		ClientOptions:  repo.ClientOptions{},
		CachingOptions: kopiaCachingOptions(cfg),
	}); err != nil {
		return fmt.Errorf("while connecting to the repository: %w", err)
	}

	// Enable ACLs
	enableACLs(ctx, kopiaBinary, configFile.Name(), cfg.EncryptionPassword)

	// Start the Kopia server
	args := []string{
		"server", "start",
		"--tls-key-file=" + cfg.TLSKey,
		"--tls-cert-file=" + cfg.TLSCert,
		"--address=" + cfg.ListenAddress,
		"--disable-file-logging",
		"--json-log-console",
	}

	// If present, add the option to use an htpasswd file for authentication
	if cfg.HTPasswdFile != "" {
		args = append(args, "--htpasswd-file="+cfg.HTPasswdFile)
	}

	kopiaServer := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaServer.Env = append(kopiaServer.Env,
		"KOPIA_CONFIG_PATH="+configFile.Name(),
		"KOPIA_CACHE_DIRECTORY="+cfg.CacheDirectory,
		"KOPIA_PASSWORD="+cfg.EncryptionPassword,
	)

	if cfg.AdminUser != "" && cfg.AdminPassword != "" {
		kopiaServer.Env = append(
			kopiaServer.Env,
			"KOPIA_SERVER_CONTROL_USER="+cfg.AdminUser,
			"KOPIA_SERVER_CONTROL_PASSWORD="+cfg.AdminPassword,
		)
	}

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
		configFile.Name(),
		cfg.CacheDirectory,
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

// kopiaCachingOptions returns the Kopia cache size for the passed configuration.
func kopiaCachingOptions(cfg *config.BaseServerConfig) content.CachingOptions {
	// https://github.com/kopia/kopia/blob/01335949d83a033ed01b29e1dd019b4379d7fb0d/cli/command_repository_connect.go#L68
	contentCacheSizeMB := int64(5000)
	metadataCacheSizeMB := int64(5000)

	// https://github.com/kopia/kopia/blob/01335949d83a033ed01b29e1dd019b4379d7fb0d/cli/command_repository_connect.go#L92
	return content.CachingOptions{
		CacheDirectory:         cfg.CacheDirectory,
		ContentCacheSizeBytes:  contentCacheSizeMB << 20,
		MetadataCacheSizeBytes: metadataCacheSizeMB << 20,
	}
}

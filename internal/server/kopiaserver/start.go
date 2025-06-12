package kopiaserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/kopia/kopia/repo/content"

	"github.com/EnterpriseDB/klio/pkg/config"
)

// kopiaCommand is the name of the kopia binary.
const kopiaCommand = "kopia"

// Start runs a Kopia server with the passed configuration.
func Start(ctx context.Context, cfg *config.BaseServerConfig) error {
	log := slog.Default()

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
			log.Warn(
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
		CachingOptions: content.CachingOptions{},
	}); err != nil {
		return fmt.Errorf("while connecting to the repository: %w", err)
	}

	// Start the Kopia server
	args := []string{
		"server", "start",
		"--tls-key-file=" + cfg.TLSKey,
		"--tls-cert-file=" + cfg.TLSCert,
		"--address=" + cfg.ListenAddress,
	}

	// If present, add the option to use an htpasswd file for authentication
	if cfg.HTPasswdFile != "" {
		args = append(args, "--htpasswd-file="+cfg.HTPasswdFile)
	}

	kopiaServer := exec.Command(kopiaBinary, args...) //nolint:gosec
	kopiaServer.Env = append(kopiaServer.Env,
		"KOPIA_CONFIG_PATH="+configFile.Name(),
		"KOPIA_LOG_DIR="+cfg.LogDirectory,
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

	log.Info("Starting Kopia server", "args", kopiaServer.Args)

	if err := kopiaServer.Start(); err != nil {
		return fmt.Errorf("while starting the kopia server: %w", err)
	}

	if err := kopiaServer.Wait(); err != nil {
		return fmt.Errorf("while running the kopia server: %w", err)
	}

	return nil
}

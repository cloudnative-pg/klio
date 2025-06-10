package kopiaserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/EnterpriseDB/klio/pkg/config"
	"github.com/kopia/kopia/repo"
	"github.com/kopia/kopia/repo/blob/filesystem"
	"github.com/kopia/kopia/repo/content"
)

// kopiaCommand is the name of the kopia binary
const kopiaCommand = "kopia"

func Start(ctx context.Context, log *slog.Logger, cfg *config.KopiaServerConfig) error {
	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	storage, err := filesystem.New(ctx, &filesystem.Options{
		Path: cfg.RepositoryDirectory,
	}, true)
	if err != nil {
		return fmt.Errorf("while creating Kopia filesystem storage: %w", err)
	}

	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	defer func() {
		if err := os.Remove(configFile.Name()); err != nil {
			slog.Warn(
				"Error while removing temporary configuration file",
				"err", err,
				"configFile", configFile.Name(),
			)
		}
	}()

	if err := repo.Connect(ctx, configFile.Name(), storage, cfg.EncryptionPassword, &repo.ConnectOptions{
		// TODO(leonardoce): these are just the default values... should
		// we set something else?
		ClientOptions:  repo.ClientOptions{},
		CachingOptions: content.CachingOptions{},
	}); err != nil {
		return err
	}

	// Let's start the Kopia server
	kopiaServer := exec.Command(
		kopiaBinary, "server", "start",
		"--tls-key-file="+cfg.TLSKey,
		"--tls-cert-file="+cfg.TLSCert,
		"--address="+cfg.ListenAddress)
	kopiaServer.Env = append(kopiaServer.Env,
		"KOPIA_CONFIG_PATH="+configFile.Name(),
		"KOPIA_LOG_DIR="+cfg.LogDirectory,
		"KOPIA_CACHE_DIRECTORY="+cfg.CacheDirectory,
		"KOPIA_PASSWORD="+cfg.EncryptionPassword,
	)
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

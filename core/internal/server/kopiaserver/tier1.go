package kopiaserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// StartTier1 runs a Tier 1 Kopia server.
func StartTier1(
	ctx context.Context,
	cfg *config.Tier1Config,
	tls *config.TLSConfig,
) error {
	contextLogger := log.FromContext(ctx)

	configFile, err := os.CreateTemp("", "kopiaconfig_*")
	if err != nil {
		return fmt.Errorf("while writing a temporary Kopia config: %w", err)
	}

	defer func() {
		if err := os.Remove(configFile.Name()); err != nil {
			contextLogger.Warning(
				"Error while removing temporary Tier 1 configuration file",
				"err", err,
				"configFile", configFile.Name(),
			)
		}
	}()

	if err := CreateTier1KopiaConfigFile(ctx, configFile.Name(), cfg); err != nil {
		return err
	}

	cacheDir, err := getTier1CacheDirectory(&cfg.Base)
	if err != nil {
		return err
	}

	kopiaCfg := Config{
		EncryptionPassword: cfg.EncryptionPassword,
		CacheDirectory:     cacheDir,
		ListenAddress:      cfg.Base.ListenAddress,
	}

	return start(ctx, configFile.Name(), &kopiaCfg, tls)
}

func getTier1CacheDirectory(cfg *config.BaseServerConfig) (string, error) {
	return cacheDirectory(cfg.CacheDirectory, "tier1")
}

// InitializeTier1 initializes a new Kopia Tier1 Repository.
func InitializeTier1(ctx context.Context, cfg *config.Tier1Config) error {
	cacheDir, err := getTier1CacheDirectory(&cfg.Base)
	if err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	if err := cleanupCache(cacheDir); err != nil {
		return err
	}

	opts := kopia.FSRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionPassword,
			CacheDirectory:     cacheDir,
		},
		DataDirectory: cfg.Base.RepositoryDirectory,
	}

	if err := kopia.InitializeFilesystem(ctx, opts); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// CreateTier1KopiaConfigFile creates a Kopia config file for tier1.
func CreateTier1KopiaConfigFile(ctx context.Context, fileName string, cfg *config.Tier1Config) error {
	cacheDir, err := getTier1CacheDirectory(&cfg.Base)
	if err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	if err := kopia.ConnectFileSystem(ctx, fileName, kopia.FSRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionPassword,
			PersistCredentials: true,
			CacheDirectory:     cacheDir,
		},
		DataDirectory: cfg.Base.RepositoryDirectory,
	}); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

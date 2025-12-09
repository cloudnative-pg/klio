package kopiaserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// StartTier1 runs a Tier 1 Kopia server.
func StartTier1(ctx context.Context, cfg *config.BaseServerConfig) error {
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

	cacheDir, err := getTier1CacheDirectory(cfg)
	if err != nil {
		return err
	}

	tier1Config := *cfg
	tier1Config.CacheDirectory = cacheDir

	return start(ctx, configFile.Name(), &tier1Config)
}

func getTier1CacheDirectory(cfg *config.BaseServerConfig) (string, error) {
	return cacheDirectory(cfg.CacheDirectory, "tier1")
}

func cleanupTier1Cache(cfg *config.BaseServerConfig) error {
	cacheDir, err := getTier1CacheDirectory(cfg)
	if err != nil {
		return err
	}

	return cleanupCache(cacheDir)
}

func getCommonTier1Args(cfg *config.BaseServerConfig) ([]string, error) {
	cacheDir, err := getTier1CacheDirectory(cfg)
	if err != nil {
		return nil, err
	}

	return []string{
		"--path=" + cfg.RepositoryDirectory,
		"--disable-file-logging",
		"--json-log-console",
		"--cache-directory=" + cacheDir,
	}, nil
}

// InitializeTier1 initializes a new Kopia Tier1 Repository.
func InitializeTier1(ctx context.Context, cfg *config.BaseServerConfig) error {
	contextLogger := log.FromContext(ctx)

	if err := cleanupTier1Cache(cfg); err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	args := []string{
		"repository", "create", "filesystem",
		"--create-only",
	}
	commonArgs, err := getCommonTier1Args(cfg)
	if err != nil {
		return err
	}
	args = append(args, commonArgs...)

	kopiaRepositoryInitialize := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaRepositoryInitialize.Env = append(kopiaRepositoryInitialize.Env,
		"KOPIA_PASSWORD="+cfg.EncryptionPassword,
	)

	kopiaRepositoryInitialize.Stdout = os.Stdout
	kopiaRepositoryInitialize.Stderr = os.Stderr

	contextLogger.Info("Kopia repository initialize", "args", kopiaRepositoryInitialize.Args)
	if err := kopiaRepositoryInitialize.Run(); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// CreateTier1KopiaConfigFile creates a Kopia config file for tier1.
func CreateTier1KopiaConfigFile(ctx context.Context, fileName string, cfg *config.BaseServerConfig) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	args := []string{
		"repository", "connect", "filesystem",
		"--config-file=" + fileName,
		"--persist-credentials",
		"--override-username=klio",
		"--override-hostname=klio",
	}
	commonArgs, err := getCommonTier1Args(cfg)
	if err != nil {
		return err
	}
	args = append(args, commonArgs...)

	kopiaRepositoryConnect := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaRepositoryConnect.Env = append(kopiaRepositoryConnect.Env,
		"KOPIA_LOG_DIR="+cfg.CacheDirectory,
		"KOPIA_PASSWORD="+cfg.EncryptionPassword,
		"KOPIA_CHECK_FOR_UPDATES=false",
	)

	kopiaRepositoryConnect.Stdout = os.Stdout
	kopiaRepositoryConnect.Stderr = os.Stderr

	contextLogger.Info("Kopia repository connect", "args", kopiaRepositoryConnect.Args)
	if err := kopiaRepositoryConnect.Run(); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

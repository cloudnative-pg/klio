package kopiaserver

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// StartTier2 runs a Tier 2 Kopia server.
func StartTier2(
	ctx context.Context,
	tier2Config *config.Tier2Config,
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
				"Error while removing temporary Tier 2 configuration file",
				"err", err,
				"configFile", configFile.Name(),
			)
		}
	}()

	if err := CreateTier2KopiaConfigFile(ctx, configFile.Name(), tier2Config); err != nil {
		return err
	}

	// A tier-2 Kopia server is configured exactly like a tier-1 server, but with
	// a different kopia target repository and different listen address.
	tier2CacheDir, err := getTier2CacheDirectory(tier2Config)
	if err != nil {
		return err
	}

	kopiaServerConfig := Config{
		EncryptionPassword: tier2Config.EncryptionPassword,
		CacheDirectory:     tier2CacheDir,
		ListenAddress:      tier2Config.BaseListenAddress,
		ReadOnly:           true,
	}

	return start(ctx, configFile.Name(), &kopiaServerConfig, tls)
}

// InitializeTier2 initializes a new Kopia Tier2 Repository.
func InitializeTier2(ctx context.Context, cfg *config.Tier2Config) error {
	contextLogger := log.FromContext(ctx)

	if err := cleanupTier2Cache(cfg); err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	args := []string{
		"repository", "create", "s3",
		"--create-only",
	}

	backendArgs, err := getCommonTier2Args(cfg)
	if err != nil {
		return err
	}

	args = append(args, backendArgs...)

	kopiaRepositoryInitialize := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaRepositoryInitialize.Env = append(kopiaRepositoryInitialize.Env, getCommonTier2Env(cfg)...)
	kopiaRepositoryInitialize.Stdout = os.Stdout
	kopiaRepositoryInitialize.Stderr = os.Stderr

	contextLogger.Info("Kopia repository initialize", "args", kopiaRepositoryInitialize.Args)
	if err := kopiaRepositoryInitialize.Run(); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// CreateTier2KopiaConfigFile creates a Kopia config file for tier2.
func CreateTier2KopiaConfigFile(ctx context.Context, fileName string, cfg *config.Tier2Config) error {
	contextLogger := log.FromContext(ctx)

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	args := []string{
		"repository", "connect", "s3",
		"--config-file=" + fileName,
		"--persist-credentials",
		"--override-username=klio",
		"--override-hostname=klio",
	}

	backendArgs, err := getCommonTier2Args(cfg)
	if err != nil {
		return err
	}

	args = append(args, backendArgs...)

	kopiaRepositoryConnect := exec.CommandContext(ctx, kopiaBinary, args...) //nolint:gosec
	kopiaRepositoryConnect.Env = append(kopiaRepositoryConnect.Env, getCommonTier2Env(cfg)...)

	kopiaRepositoryConnect.Stdout = os.Stdout
	kopiaRepositoryConnect.Stderr = os.Stderr

	contextLogger.Info("Kopia repository connect", "args", kopiaRepositoryConnect.Args)
	if err := kopiaRepositoryConnect.Run(); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

func getTier2CacheDirectory(cfg *config.Tier2Config) (string, error) {
	return cacheDirectory(cfg.CacheDirectory, "tier2")
}

func cleanupTier2Cache(cfg *config.Tier2Config) error {
	cacheDir, err := getTier2CacheDirectory(cfg)
	if err != nil {
		return err
	}

	return cleanupCache(cacheDir)
}

func getCommonTier2Args(cfg *config.Tier2Config) ([]string, error) {
	doNotUseTLS := false
	shortenedEndpoint := ""

	if cfg.S3.Endpoint != "" {
		endpointURL, err := url.Parse(cfg.S3.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint URL %q: %w", endpointURL, err)
		}

		doNotUseTLS = strings.ToLower(endpointURL.Scheme) != "https"
		shortenedEndpoint = endpointURL.Host
	}

	cacheDir, err := getTier2CacheDirectory(cfg)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--bucket=" + cfg.S3.BucketName,
		"--cache-directory=" + cacheDir,
		"--prefix=" + path.Join(cfg.S3.Prefix, "base") + "/",
		"--disable-file-logging",
		"--json-log-console",
	}

	if cfg.S3.Region != "" {
		args = append(args, "--region="+cfg.S3.Region)
	}

	if shortenedEndpoint != "" {
		args = append(args, "--endpoint="+shortenedEndpoint)
	}
	if doNotUseTLS {
		args = append(args, "--disable-tls")
	}
	if cfg.S3.CustomCABundleFile != "" {
		args = append(args, "--root-ca-pem-path="+cfg.S3.CustomCABundleFile)
	}

	return args, nil
}

func getCommonTier2Env(cfg *config.Tier2Config) []string {
	return []string{
		"KOPIA_LOG_DIR=" + cfg.CacheDirectory,
		"KOPIA_PASSWORD=" + cfg.EncryptionPassword,
		"AWS_ACCESS_KEY_ID=" + cfg.S3.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + cfg.S3.SecretAccessKey,
		"AWS_SESSION_TOKEN=" + cfg.S3.SessionToken,
		"KOPIA_CHECK_FOR_UPDATES=false",
	}
}

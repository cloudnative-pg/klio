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

// StartTier2 runs a Tier 2 Kopia server.
func StartTier2(
	ctx context.Context,
	tier2Config *config.Tier2Config,
	tls *config.TLSConfig,
	serverControl ServerControlCredential,
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
		CacheDirectory:        tier2CacheDir,
		ListenAddress:         tier2Config.BaseListenAddress,
		ReadOnly:              true,
		ServerControlUser:     serverControl.User,
		ServerControlPassword: serverControl.Password,
	}

	return start(ctx, configFile.Name(), &kopiaServerConfig, tls)
}

// InitializeTier2 initializes a new Kopia Tier2 Repository.
func InitializeTier2(ctx context.Context, cfg *config.Tier2Config) error {
	cacheDir, err := getTier2CacheDirectory(cfg)
	if err != nil {
		return err
	}

	if err := cleanupCache(cacheDir); err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	if err := kopia.InitializeS3(ctx, kopia.S3RepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			PersistCredentials: false,
			CacheDirectory:     cacheDir,
		},
		BucketName:         cfg.S3.BucketName,
		Endpoint:           cfg.S3.Endpoint,
		Region:             cfg.S3.Region,
		Prefix:             cfg.S3.Prefix,
		AccessKeyID:        cfg.S3.AccessKeyID,
		SecretAccessKey:    cfg.S3.SecretAccessKey,
		SessionToken:       cfg.S3.SessionToken,
		CustomCABundleFile: cfg.S3.CustomCABundleFile,
	}); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

// CreateTier2KopiaConfigFile creates a Kopia config file for tier2.
func CreateTier2KopiaConfigFile(ctx context.Context, fileName string, cfg *config.Tier2Config) error {
	cacheDir, err := getTier2CacheDirectory(cfg)
	if err != nil {
		return err
	}

	kopiaBinary, err := exec.LookPath(kopiaCommand)
	if err != nil {
		return fmt.Errorf("kopia binary not found (%q): %w", kopiaCommand, err)
	}

	if err := kopia.ConnectS3(ctx, fileName, kopia.S3RepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			PersistCredentials: true,
			CacheDirectory:     cacheDir,
		},
		BucketName:         cfg.S3.BucketName,
		Endpoint:           cfg.S3.Endpoint,
		Region:             cfg.S3.Region,
		Prefix:             cfg.S3.Prefix,
		AccessKeyID:        cfg.S3.AccessKeyID,
		SecretAccessKey:    cfg.S3.SecretAccessKey,
		SessionToken:       cfg.S3.SessionToken,
		CustomCABundleFile: cfg.S3.CustomCABundleFile,
	}); err != nil {
		return fmt.Errorf("while connecting to Kopia repository: %w", err)
	}

	return nil
}

func getTier2CacheDirectory(cfg *config.Tier2Config) (string, error) {
	return cacheDirectory(cfg.CacheDirectory, "tier2")
}

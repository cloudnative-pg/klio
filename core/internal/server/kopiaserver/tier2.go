package kopiaserver

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// StartTier2 runs a Tier 2 Kopia server.
func StartTier2(
	ctx context.Context,
	listenAddress string,
	tls *config.TLSConfig,
	serverControl ServerControlCredential,
	kopiaConfigFile string,
) error {
	kopiaServerConfig := Config{
		ListenAddress:         listenAddress,
		ReadOnly:              true,
		ServerControlUser:     serverControl.User,
		ServerControlPassword: serverControl.Password,
	}

	return start(ctx, kopiaConfigFile, &kopiaServerConfig, tls)
}

// InitializeTier2 initializes a new Kopia Tier2 Repository.
func InitializeTier2(ctx context.Context, cfg *config.Tier2Config) error {
	cacheDir := cfg.CacheDirectory
	if err := kopia.CleanupCacheDirectory(cacheDir); err != nil {
		return err
	}

	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
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

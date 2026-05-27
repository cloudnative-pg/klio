package kopiaserver

import (
	"context"
	"fmt"

	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// ServerControlCredential contains the credentials for Kopia server control operations.
type ServerControlCredential struct {
	// User is the username for server control authentication.
	User string

	// Password is the password for server control authentication.
	Password string
}

// StartTier1 runs a Tier 1 Kopia server.
func StartTier1(
	ctx context.Context,
	listenAddress string,
	tls *config.TLSConfig,
	serverControl ServerControlCredential,
	kopiaConfigFile string,
) error {
	kopiaCfg := Config{
		ListenAddress:         listenAddress,
		ServerControlUser:     serverControl.User,
		ServerControlPassword: serverControl.Password,
	}

	return start(ctx, kopiaConfigFile, &kopiaCfg, tls, opentelemetry.Tier1)
}

// InitializeTier1 initializes a new Kopia Tier1 Repository.
func InitializeTier1(ctx context.Context, cfg *config.Tier1Config) error {
	cacheDir := cfg.Base.CacheDirectory
	kopiaBinary, err := kopia.LookupBinary()
	if err != nil {
		return err
	}

	if err := kopia.CleanupCacheDirectory(cacheDir); err != nil {
		return err
	}

	opts := kopia.FSRepoOpts{
		CommonRepoOpts: kopia.CommonRepoOpts{
			KopiaBinary:        kopiaBinary,
			EncryptionPassword: cfg.EncryptionKey,
			CacheDirectory:     cacheDir,
		},
		DataDirectory: cfg.Base.RepositoryDirectory,
	}

	if err := kopia.InitializeFilesystem(ctx, opts); err != nil {
		return fmt.Errorf("while creating Kopia repository: %w", err)
	}

	return nil
}

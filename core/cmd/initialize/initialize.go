/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package initialize

import (
	"context"
	"fmt"

	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/cloudnative-pg/klio/core/internal/repository"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaconfig"
	"github.com/cloudnative-pg/klio/core/internal/server/kopiaserver"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

// KopiaOptions contains all the options needed to initialize and verify a Kopia repository.
type KopiaOptions struct {
	// The Kopia FS
	FS afero.Fs

	// The Kopia encryption password
	EncryptionPassword string

	// InitializeRepo is called to create the Kopia repository
	InitializeRepo func(ctx context.Context) error

	// VerifyRepo is called to verify the Kopia repository password
	VerifyRepo func(ctx context.Context) error
}

// Options are the options needed to initialize a new
// pair of repositories.
type Options struct {
	// The WAL FS
	WalFS afero.Fs

	// The encryption password to be used for the WALs
	WalEncryptionPassword string

	// Kopia contains all the options for the Kopia repository
	Kopia *KopiaOptions
}

// NewTier1Options creates Options for Tier1 initialization.
func NewTier1Options(cfg *config.Tier1Config) Options {
	walDirectory := cfg.Wal.WALPath
	kopiaDirectory := cfg.Base.RepositoryDirectory

	return Options{
		WalFS:                 afero.NewBasePathFs(afero.NewOsFs(), walDirectory),
		WalEncryptionPassword: cfg.EncryptionKey,
		Kopia: &KopiaOptions{
			FS:                 afero.NewBasePathFs(afero.NewOsFs(), kopiaDirectory),
			EncryptionPassword: cfg.EncryptionKey,
			InitializeRepo: func(ctx context.Context) error {
				return kopiaserver.InitializeTier1(ctx, cfg)
			},
			VerifyRepo: func(ctx context.Context) error {
				return kopiaconfig.VerifyTier1KopiaRepository(ctx, cfg)
			},
		},
	}
}

// NewTier2Options creates Options for Tier2 initialization.
func NewTier2Options(cfg *config.Tier2Config, walFS, kopiaFS afero.Fs) Options {
	return Options{
		WalFS:                 walFS,
		WalEncryptionPassword: cfg.EncryptionKey,
		Kopia: &KopiaOptions{
			FS:                 kopiaFS,
			EncryptionPassword: cfg.EncryptionKey,
			InitializeRepo: func(ctx context.Context) error {
				return kopiaserver.InitializeTier2(ctx, cfg)
			},
			VerifyRepo: func(ctx context.Context) error {
				return kopiaconfig.VerifyTier2KopiaRepository(ctx, cfg)
			},
		},
	}
}

// verifyExistingRepositories verifies passwords for any existing repositories.
func verifyExistingRepositories(ctx context.Context, opts Options) error {
	contextLogger := log.FromContext(ctx)

	walDirectoryIsEmpty, err := canInitRepoDirectory(opts.WalFS)
	if err != nil {
		return fmt.Errorf("while checking if the Klio WAL FS is safe to use: %w", err)
	}

	kopiaDirectoryIsEmpty, err := canInitRepoDirectory(opts.Kopia.FS)
	if err != nil {
		return fmt.Errorf("while checking if the Kopia repository is safe to use: %w", err)
	}

	if !walDirectoryIsEmpty {
		contextLogger.Info("WAL repository exists, verifying password")
		walConn, err := repository.Open(repository.Options{
			FS:       opts.WalFS,
			Password: opts.WalEncryptionPassword,
		})
		if err != nil {
			return fmt.Errorf("WAL repository exists but password verification failed: %w", err)
		}
		walConn.Close()
		contextLogger.Info("WAL repository password verified successfully")
	}

	if !kopiaDirectoryIsEmpty {
		contextLogger.Info("Kopia repository exists, verifying password")
		if err := opts.Kopia.VerifyRepo(ctx); err != nil {
			return fmt.Errorf("kopia repository exists but password verification failed: %w", err)
		}
		contextLogger.Info("Kopia repository password verified successfully")
	}

	return nil
}

// initializeMissingRepositories creates any missing repositories.
func initializeMissingRepositories(ctx context.Context, opts Options) error {
	contextLogger := log.FromContext(ctx)

	walDirectoryIsEmpty, err := canInitRepoDirectory(opts.WalFS)
	if err != nil {
		return fmt.Errorf("while checking if the Klio WAL FS is safe to use: %w", err)
	}

	kopiaDirectoryIsEmpty, err := canInitRepoDirectory(opts.Kopia.FS)
	if err != nil {
		return fmt.Errorf("while checking if the Kopia repository is safe to use: %w", err)
	}

	if walDirectoryIsEmpty {
		contextLogger.Info("WAL repository does not exist, initializing")
		if err := repository.Initialize(repository.Options{
			FS:       opts.WalFS,
			Password: opts.WalEncryptionPassword,
		}); err != nil {
			return fmt.Errorf("while initializing the Klio WAL directory, %w", err)
		}
		contextLogger.Info("WAL repository initialized successfully")
	}

	if kopiaDirectoryIsEmpty {
		contextLogger.Info("Kopia repository does not exist, initializing")
		if err := opts.Kopia.InitializeRepo(ctx); err != nil {
			return fmt.Errorf("while initializing the Kopia repository directory, %w", err)
		}
		contextLogger.Info("Kopia repository initialized successfully")
	}

	if !walDirectoryIsEmpty && !kopiaDirectoryIsEmpty {
		contextLogger.Info("Both repositories already exist and passwords verified, nothing to initialize")
	}

	return nil
}

// Run initializes the Klio WAL and the Kopia repository specified by the
// options.
func Run(ctx context.Context, opts Options) error {
	// Phase 1: Verify existing components
	if err := verifyExistingRepositories(ctx, opts); err != nil {
		return err
	}

	// Phase 2: Create missing components
	return initializeMissingRepositories(ctx, opts)
}

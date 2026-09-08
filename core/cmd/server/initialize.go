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

package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/cmd/initialize"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func initializeRepository(ctx context.Context, opts serverOpts) error {
	if opts.tier1 {
		if err := initializeTier1(ctx, opts.fs, opts.cfg); err != nil {
			return err
		}
	}

	if opts.tier2 {
		if err := initializeTier2(ctx, opts.fs, opts.cfg); err != nil {
			return err
		}
	}

	return nil
}

func initializeTier1(ctx context.Context, fs afero.Fs, cfg *config.ServerConfig) error {
	log.FromContext(ctx).Info(
		"Ensuring tier1 repository is initialized.",
		"walDirectory", cfg.Tier1.Wal.WALPath,
		"kopiaDirectory", cfg.Tier1.Base.RepositoryDirectory,
		"cacheDirectory", cfg.Tier1.Base.CacheDirectory,
		"queueDirectory", cfg.QueueDirectory,
	)

	// The queue is always required when tier1 is enabled to support retention
	// policy enforcement. Retention needs to check for pending tier2 transfers
	// even when tier2 is not configured, to allow consistent behavior across
	// configurations.
	if cfg.QueueDirectory == "" {
		return errors.New("queue is required when tier1 is enabled")
	}

	if err := fs.MkdirAll(cfg.QueueDirectory, 0o750); err != nil {
		return fmt.Errorf("while ensuring that the queue directory exists: %w", err)
	}

	if err := fs.MkdirAll(cfg.Tier1.Wal.WALPath, 0o750); err != nil {
		return fmt.Errorf("while ensuring that the tier1 WAL directory exists: %w", err)
	}

	if err := fs.MkdirAll(cfg.Tier1.Base.RepositoryDirectory, 0o750); err != nil {
		return fmt.Errorf("while ensuring that the tier1 repository directory exists: %w", err)
	}

	if err := fs.MkdirAll(cfg.Tier1.Base.CacheDirectory, 0o750); err != nil {
		return fmt.Errorf("while ensuring that the tier1 cache directory exists: %w", err)
	}

	return initialize.Run(ctx, initialize.NewTier1Options(fs, &cfg.Tier1))
}

func initializeTier2(ctx context.Context, fs afero.Fs, cfg *config.ServerConfig) error {
	if err := fs.MkdirAll(cfg.Tier2.CacheDirectory, 0o750); err != nil {
		return fmt.Errorf("while ensuring that the tier2 cache directory exists: %w", err)
	}

	tier2BaseFS, err := tier2.ConnectBase(ctx, &cfg.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2 (base): %w", err)
	}

	tier2WALFS, err := tier2.ConnectWAL(ctx, &cfg.Tier2)
	if err != nil {
		return fmt.Errorf("error while connecting to tier2 (wal): %w", err)
	}

	log.FromContext(ctx).Info("Ensuring tier2 repository is initialized.")

	return initialize.Run(ctx, initialize.NewTier2Options(&cfg.Tier2, tier2WALFS, tier2BaseFS))
}

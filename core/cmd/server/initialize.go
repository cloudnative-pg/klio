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
	"fmt"
	"path/filepath"

	"github.com/cloudnative-pg/machinery/pkg/log"

	"github.com/cloudnative-pg/klio/core/cmd/initialize"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
	"github.com/cloudnative-pg/klio/core/internal/tier2"
	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func initializeRepository(ctx context.Context, opts serverOpts) error {
	if opts.tier1 {
		if err := initializeTier1(ctx, opts.cfg); err != nil {
			return err
		}
	}

	if opts.tier2 {
		if err := initializeTier2(ctx, opts.cfg); err != nil {
			return err
		}
	}

	return nil
}

// reclaimStaleCache removes a Kopia cache directory left behind on another
// volume by a cache that moved. Kopia rebuilds the cache on demand, so the
// leftover is only wasted space.
func reclaimStaleCache(ctx context.Context, stale, inUse string) error {
	if stale == "" || filepath.Clean(stale) == filepath.Clean(inUse) {
		return nil
	}

	log.FromContext(ctx).Info("Reclaiming stale Kopia cache directory", "directory", stale)

	return kopia.CleanupCacheDirectory(stale)
}

func initializeTier1(ctx context.Context, cfg *config.ServerConfig) error {
	if err := reclaimStaleCache(
		ctx, cfg.Tier1.Base.StaleCacheDirectory, cfg.Tier1.Base.CacheDirectory,
	); err != nil {
		return err
	}

	walDirectory := cfg.Tier1.Wal.WALPath
	kopiaDirectory := cfg.Tier1.Base.RepositoryDirectory

	log.FromContext(ctx).Info(
		"Ensuring tier1 repository is initialized.",
		"walDirectory", walDirectory,
		"kopiaDirectory", kopiaDirectory,
	)

	return initialize.Run(ctx, initialize.NewTier1Options(&cfg.Tier1))
}

func initializeTier2(ctx context.Context, cfg *config.ServerConfig) error {
	if err := reclaimStaleCache(
		ctx, cfg.Tier2.StaleCacheDirectory, cfg.Tier2.CacheDirectory,
	); err != nil {
		return err
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

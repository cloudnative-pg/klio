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
	"testing"

	"github.com/spf13/afero"

	"github.com/cloudnative-pg/klio/core/pkg/config"
)

func assertDirExists(t *testing.T, fs afero.Fs, dir string) {
	t.Helper()

	ok, err := afero.DirExists(fs, dir)
	if err != nil {
		t.Fatalf("while checking if directory %q exists: %v", dir, err)
	}
	if !ok {
		t.Fatalf("expected directory %q to exist", dir)
	}
}

func assertDirNotExists(t *testing.T, fs afero.Fs, dir string) {
	t.Helper()

	ok, err := afero.DirExists(fs, dir)
	if err != nil {
		t.Fatalf("while checking if directory %q exists: %v", dir, err)
	}
	if ok {
		t.Fatalf("expected directory %q not to exist", dir)
	}
}

func newTestServerConfig() *config.ServerConfig {
	return &config.ServerConfig{
		QueueDirectory: "/queue",
		Tier1: config.Tier1Config{
			Base: config.BaseServerConfig{
				CacheDirectory:      "/cache_tier1/kopia-cache",
				RepositoryDirectory: "/data/base",
			},
			Wal: config.WalServerConfig{
				WALPath: "/data/wal",
			},
		},
		Tier2: config.Tier2Config{
			CacheDirectory: "/cache_tier2/kopia-cache",
		},
	}
}

// TestInitializeTier1CreatesDirectories verifies that initializeTier1
// creates the queue, WAL, Kopia repository, and cache directories, and does
// not touch the tier2 one, even though it later fails (there is no real
// Kopia repository backing the freshly created, empty directories in the
// test environment).
func TestInitializeTier1CreatesDirectories(t *testing.T) {
	cfg := newTestServerConfig()
	fs := afero.NewMemMapFs()

	_ = initializeTier1(context.Background(), fs, cfg)

	assertDirExists(t, fs, cfg.QueueDirectory)
	assertDirExists(t, fs, cfg.Tier1.Wal.WALPath)
	assertDirExists(t, fs, cfg.Tier1.Base.RepositoryDirectory)
	assertDirExists(t, fs, cfg.Tier1.Base.CacheDirectory)
	assertDirNotExists(t, fs, cfg.Tier2.CacheDirectory)
}

// TestInitializeTier2CreatesDirectories verifies that initializeTier2
// creates only the tier2 cache directory, and none of the tier1 ones (nor
// the queue directory, which is only needed when tier1 is enabled), even
// though it later fails (there is no real tier2/S3 backend in the test
// environment).
func TestInitializeTier2CreatesDirectories(t *testing.T) {
	cfg := newTestServerConfig()
	fs := afero.NewMemMapFs()

	_ = initializeTier2(context.Background(), fs, cfg)

	assertDirExists(t, fs, cfg.Tier2.CacheDirectory)
	assertDirNotExists(t, fs, cfg.QueueDirectory)
	assertDirNotExists(t, fs, cfg.Tier1.Wal.WALPath)
	assertDirNotExists(t, fs, cfg.Tier1.Base.RepositoryDirectory)
	assertDirNotExists(t, fs, cfg.Tier1.Base.CacheDirectory)
}

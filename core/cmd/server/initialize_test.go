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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCacheDirectory(t *testing.T, parent, name string) string {
	t.Helper()

	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blob"), []byte("cached"), 0o600))

	return dir
}

func TestReclaimStaleCacheRemovesTheUnusedDirectory(t *testing.T) {
	root := t.TempDir()
	stale := newCacheDirectory(t, root, "stale")
	inUse := newCacheDirectory(t, root, "in-use")

	require.NoError(t, reclaimStaleCache(context.Background(), stale, inUse))

	assert.NoDirExists(t, stale)
	assert.DirExists(t, inUse)
}

func TestReclaimStaleCacheKeepsTheDirectoryInUse(t *testing.T) {
	root := t.TempDir()
	inUse := newCacheDirectory(t, root, "in-use")

	// The two paths point at the same directory, spelled differently.
	require.NoError(t, reclaimStaleCache(context.Background(), inUse+"/", inUse))

	assert.DirExists(t, inUse)
}

func TestReclaimStaleCacheIsANoOpWhenUnset(t *testing.T) {
	inUse := newCacheDirectory(t, t.TempDir(), "in-use")

	require.NoError(t, reclaimStaleCache(context.Background(), "", inUse))

	assert.DirExists(t, inUse)
}

func TestReclaimStaleCacheRefusesRelativePaths(t *testing.T) {
	require.Error(t, reclaimStaleCache(context.Background(), "cache", "/data/cache"))
}

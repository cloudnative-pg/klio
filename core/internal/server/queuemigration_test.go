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
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bindMount makes path a mount point of its own content, the same way
// Kubernetes mounts a PVC at a container path with no SubPath. It is what
// turns a plain directory into something os.RemoveAll can no longer rmdir.
// The test using it is skipped, rather than failed, when the sandbox running
// it cannot create mounts.
func bindMount(t *testing.T, path string) {
	t.Helper()

	if err := syscall.Mount(path, path, "", syscall.MS_BIND, ""); err != nil {
		t.Skipf("skipping: cannot create a bind mount in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Unmount(path, 0)
	})
}

// writeTree creates the given files, whose paths are relative to root, with
// their content as body.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		target := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o750))
		require.NoError(t, os.WriteFile(target, []byte(content), 0o600))
	}
}

// readTree returns every regular file under root, keyed by its path relative
// to root.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()

	result := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(root, path)
		require.NoError(t, err)

		content, err := os.ReadFile(path) //nolint:gosec // the path comes from the walk of a test directory
		require.NoError(t, err)
		result[relativePath] = string(content)

		return nil
	}))

	return result
}

func TestMigrateQueueDirectory(t *testing.T) {
	queueFiles := map[string]string{
		"jetstream/$G/streams/klio-wal-stream/meta.inf":   "meta",
		"jetstream/$G/streams/klio-wal-stream/msgs/1.blk": "block",
	}

	// The destination directory already exists, as the volume it lives on is
	// mounted.
	t.Run("moves the queue to the new location", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "queue")
		destination := filepath.Join(base, "data", "queue")
		writeTree(t, source, queueFiles)
		require.NoError(t, os.MkdirAll(destination, 0o750))

		require.NoError(t, MigrateQueueDirectory(context.Background(), source, destination))

		assert.Equal(t, queueFiles, readTree(t, destination))
		_, err := os.Stat(source)
		assert.True(t, os.IsNotExist(err), "the previous location must be removed")
	})

	t.Run("keeps the destination when both locations hold a queue", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "queue")
		destination := filepath.Join(base, "data", "queue")
		writeTree(t, source, queueFiles)
		writeTree(t, destination, map[string]string{"jetstream/current.inf": "current"})

		require.NoError(t, MigrateQueueDirectory(context.Background(), source, destination))

		assert.Equal(t, map[string]string{"jetstream/current.inf": "current"}, readTree(t, destination))
		assert.Equal(t, queueFiles, readTree(t, source), "the previous location must be left untouched")
	})

	t.Run("recovers from an interrupted migration", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "queue")
		destination := filepath.Join(base, "data", "queue")
		writeTree(t, source, queueFiles)
		// A previous attempt died halfway through the copy.
		staging := filepath.Join(destination, migrationStagingName)
		writeTree(t, staging, map[string]string{"jetstream/truncated.inf": "partial"})

		require.NoError(t, MigrateQueueDirectory(context.Background(), source, destination))

		assert.Equal(t, queueFiles, readTree(t, destination))
		_, err := os.Stat(staging)
		assert.True(t, os.IsNotExist(err), "the staging directory must be gone")
	})

	// Reproduces adding a dedicated queue volume to a server that was storing
	// the queue inside the tier1 data volume: the operator mounts the new PVC
	// at destination with no SubPath, so destination is a mount point, not a
	// plain subdirectory as in the other cases above.
	t.Run("moves the queue into a freshly mounted destination", func(t *testing.T) {
		base := t.TempDir()
		source := filepath.Join(base, "data", "queue")
		destination := filepath.Join(base, "queue")
		writeTree(t, source, queueFiles)
		require.NoError(t, os.MkdirAll(destination, 0o750))
		bindMount(t, destination)

		require.NoError(t, MigrateQueueDirectory(context.Background(), source, destination))

		assert.Equal(t, queueFiles, readTree(t, destination))
	})

	t.Run("is a no-op when there is nothing to migrate", func(t *testing.T) {
		base := t.TempDir()
		destination := filepath.Join(base, "queue")
		writeTree(t, destination, queueFiles)

		require.NoError(t, MigrateQueueDirectory(context.Background(), "", destination))
		require.NoError(t, MigrateQueueDirectory(context.Background(), filepath.Join(base, "missing"), destination))
		require.NoError(t, MigrateQueueDirectory(context.Background(), destination, destination))

		assert.Equal(t, queueFiles, readTree(t, destination))
	})
}

func TestIsEmptyDir(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
		want  bool
	}{
		{
			name:  "missing path",
			setup: func(_ *testing.T, _ string) {},
			want:  true,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T, path string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(path, 0o750))
			},
			want: true,
		},
		{
			name: "holds only a leftover migration staging directory",
			setup: func(t *testing.T, path string) {
				t.Helper()
				writeTree(t, filepath.Join(path, migrationStagingName), map[string]string{"f": "content"})
			},
			want: true,
		},
		{
			name: "holds a leftover staging directory and real queue content",
			setup: func(t *testing.T, path string) {
				t.Helper()
				writeTree(t, filepath.Join(path, migrationStagingName), map[string]string{"f": "content"})
				writeTree(t, path, map[string]string{"jetstream/current.inf": "current"})
			},
			want: false,
		},
		{
			name: "holds real content",
			setup: func(t *testing.T, path string) {
				t.Helper()
				writeTree(t, path, map[string]string{"jetstream/current.inf": "current"})
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "dir")
			test.setup(t, path)

			got, err := isEmptyDir(path)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestAdoptStagedQueue(t *testing.T) {
	t.Run("moves every top-level entry into destination", func(t *testing.T) {
		base := t.TempDir()
		staging := filepath.Join(base, "staging")
		destination := filepath.Join(base, "destination")
		writeTree(t, staging, map[string]string{
			"jetstream/$G/streams/klio-wal-stream/meta.inf": "meta",
			"top-level-file.txt":                            "content",
		})
		require.NoError(t, os.MkdirAll(destination, 0o750))

		require.NoError(t, adoptStagedQueue(staging, destination))

		assert.Equal(t, map[string]string{
			"jetstream/$G/streams/klio-wal-stream/meta.inf": "meta",
			"top-level-file.txt":                            "content",
		}, readTree(t, destination))
		entries, err := os.ReadDir(staging)
		require.NoError(t, err)
		assert.Empty(t, entries, "staging must be left empty, its entries moved out")
	})

	t.Run("stops at the first entry it cannot move", func(t *testing.T) {
		base := t.TempDir()
		staging := filepath.Join(base, "staging")
		destination := filepath.Join(base, "destination")
		writeTree(t, staging, map[string]string{"conflicting/file.txt": "new"})
		// A non-empty directory already sits where "conflicting" would move to:
		// os.Rename refuses to replace it.
		writeTree(t, filepath.Join(destination, "conflicting"), map[string]string{"other.txt": "old"})

		err := adoptStagedQueue(staging, destination)

		require.Error(t, err)
		assert.Equal(t, map[string]string{"conflicting/other.txt": "old"}, readTree(t, destination),
			"the pre-existing destination content must be left untouched")
	})
}

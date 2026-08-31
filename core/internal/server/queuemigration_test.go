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

	// The destination directory already exists: the volume it lives on is
	// mounted, and the rename has to replace it.
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
		writeTree(t, destination+migrationSuffix, map[string]string{"jetstream/truncated.inf": "partial"})

		require.NoError(t, MigrateQueueDirectory(context.Background(), source, destination))

		assert.Equal(t, queueFiles, readTree(t, destination))
		_, err := os.Stat(destination + migrationSuffix)
		assert.True(t, os.IsNotExist(err), "the staging directory must be gone")
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

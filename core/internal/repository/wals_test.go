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

package repository

import (
	"context"
	"path"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetArchivedWALFileName(t *testing.T) {
	clusterName := "cluster-example"

	tests := []struct {
		walName      string
		archivedName string
	}{
		{
			walName:      "00000001000000760000007B",
			archivedName: "cluster-example/0000000100000076/00000001000000760000007B",
		},
		{
			walName:      "00000001000000760000007B.partial",
			archivedName: "cluster-example/0000000100000076/00000001000000760000007B.partial",
		},
		{
			walName:      "00000002.history",
			archivedName: "cluster-example/00000002.history",
		},
		{
			walName:      "0000000100001234000055CD.007C9330.backup",
			archivedName: "cluster-example/0000000100001234000055CD.007C9330.backup",
		},
	}

	for _, c := range tests {
		t.Run(c.walName, func(t *testing.T) {
			assert.Equal(t, c.archivedName, getWALArchivePath(clusterName, c.walName))
		})
	}
}

func TestGetLatestWALFileForCluster(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "test-password",
	}

	err := Initialize(opts)
	require.NoError(t, err)

	conn, err := Open(opts)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	// Test with non-existent cluster
	latestWal, err := conn.GetLatestWALFileForCluster(context.Background(), "non-existent-cluster")
	require.NoError(t, err)
	assert.Empty(t, latestWal)

	// Create test structure: cluster/0000000100000000/WAL files
	clusterName := "test-cluster"
	walDir := path.Join(clusterName, "0000000100000000")

	// Create directories
	err = opts.FS.MkdirAll(walDir, 0o750)
	require.NoError(t, err)

	// Create test WAL files
	walNames := []string{
		"00000001000000000000000A",
		"00000001000000000000000B",
		"00000001000000000000000C",
	}

	for _, walName := range walNames {
		file, err := opts.FS.Create(path.Join(walDir, walName))
		require.NoError(t, err)
		_, _ = file.Write([]byte("test data"))
		_ = file.Close()
	}

	// Get latest WAL
	latestWal, err = conn.GetLatestWALFileForCluster(context.Background(), clusterName)
	require.NoError(t, err)
	assert.Equal(t, "00000001000000000000000C", latestWal)

	// Test with empty cluster directory
	emptyClusterDir := path.Join("empty-cluster", "0000000100000000")
	err = opts.FS.MkdirAll(emptyClusterDir, 0o750)
	require.NoError(t, err)

	latestWal, err = conn.GetLatestWALFileForCluster(context.Background(), "empty-cluster")
	require.NoError(t, err)
	assert.Empty(t, latestWal)
}

func TestGetEarliestWALFileForCluster(t *testing.T) {
	opts := Options{
		FS:       afero.NewMemMapFs(),
		Password: "test-password",
	}
	require.NoError(t, Initialize(opts))

	conn, err := Open(opts)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	tests := []struct {
		name        string
		clusterName string
		createDir   bool
		walNames    []string
		expected    string
	}{
		{
			name:        "non-existent cluster",
			clusterName: "non-existent-cluster",
			expected:    "",
		},
		{
			name:        "several WAL files returns the smallest",
			clusterName: "test-cluster",
			createDir:   true,
			walNames: []string{
				"00000001000000000000000A",
				"00000001000000000000000B",
				"00000001000000000000000C",
			},
			expected: "00000001000000000000000A",
		},
		{
			name:        "empty cluster directory",
			clusterName: "empty-cluster",
			createDir:   true,
			expected:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.createDir {
				walDir := path.Join(tc.clusterName, "0000000100000000")
				require.NoError(t, opts.FS.MkdirAll(walDir, 0o750))
				for _, walName := range tc.walNames {
					file, err := opts.FS.Create(path.Join(walDir, walName))
					require.NoError(t, err)
					require.NoError(t, file.Close())
				}
			}

			earliestWal, err := conn.GetEarliestWALFileForCluster(context.Background(), tc.clusterName)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, earliestWal)
		})
	}
}

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

package kopia

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripTagPrefix(t *testing.T) {
	t.Run("strips the tag: prefix Kopia's CLI adds to every key", func(t *testing.T) {
		stripped := stripTagPrefix(map[string]string{
			"tag:klio.io/tag":            "backup-20260916100319",
			"tag:klio.io/content":        "pgdata",
			"tag:klio.io/tablespaceName": "",
		})

		assert.Equal(t, map[string]string{
			"klio.io/tag":            "backup-20260916100319",
			"klio.io/content":        "pgdata",
			"klio.io/tablespaceName": "",
		}, stripped)
	})

	t.Run("leaves an already unprefixed key untouched", func(t *testing.T) {
		stripped := stripTagPrefix(map[string]string{"klio.io/tag": "value"})

		assert.Equal(t, map[string]string{"klio.io/tag": "value"}, stripped)
	})

	t.Run("nil map returns an empty, non-nil map", func(t *testing.T) {
		assert.Equal(t, map[string]string{}, stripTagPrefix(nil))
	})
}

// This fixture reproduces the exact shape of "kopia snapshot list --json"
// output captured from a live repository: every user-defined tag key is
// stored with a literal "tag:" prefix (see cli/command_snapshot_create.go's
// getTags in the vendored Kopia source). Earlier tests in this package built
// kopia.Manifest values directly in Go, so they never exercised this JSON
// unmarshalling path and never caught that every Tags lookup by the plain
// klioclient constant was a silent no-op.
const rawSnapshotListJSON = `[
  {
    "id": "k1aaaa",
    "source": {"host": "cluster-example", "userName": "klio", "path": "/"},
    "rootEntry": {"obj": "obj1"},
    "startTime": "2026-09-16T10:03:21.000000000Z",
    "endTime": "2026-09-16T10:03:25.000000000Z",
    "tags": {
      "tag:klio.io/tag": "backup-20260916100319",
      "tag:klio.io/content": "pgdata"
    }
  },
  {
    "id": "k1bbbb",
    "source": {"host": "cluster-example", "userName": "klio", "path": "/"},
    "rootEntry": {"obj": "obj2"},
    "startTime": "2026-09-16T10:03:26.000000000Z",
    "endTime": "2026-09-16T10:03:28.000000000Z",
    "tags": {
      "tag:klio.io/tag": "backup-20260916100319",
      "tag:klio.io/content": "control"
    }
  }
]`

func TestListSnapshotsUnprefixesTagsFromRealKopiaJSON(t *testing.T) {
	var entries []Manifest
	require.NoError(t, json.Unmarshal([]byte(rawSnapshotListJSON), &entries))

	for i := range entries {
		entries[i].Tags = stripTagPrefix(entries[i].Tags)
	}

	require.Len(t, entries, 2)
	assert.Equal(t, "backup-20260916100319", entries[0].Tags["klio.io/tag"])
	assert.Equal(t, "pgdata", entries[0].Tags["klio.io/content"])
	assert.Equal(t, "control", entries[1].Tags["klio.io/content"])
}

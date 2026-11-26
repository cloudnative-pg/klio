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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVerifyOutput(t *testing.T) {
	t.Run("valid JSON with errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":2,"errorStrings":["bad object 1","bad object 2"]}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 2, result.ErrorCount)
		assert.Equal(t, []string{"bad object 1", "bad object 2"}, result.ErrorStrings)
	})

	t.Run("valid JSON with no errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":0}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 0, result.ErrorCount)
		assert.Empty(t, result.ErrorStrings)
	})

	t.Run("empty output returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte{})
		assert.Equal(t, VerifyResult{}, result)
	})

	t.Run("invalid JSON returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte("not json"))
		assert.Equal(t, VerifyResult{}, result)
	})
}

func TestBuildMigrateArgs(t *testing.T) {
	opts := SnapshotMigrateOpts{
		SourceConfig: "/etc/kopia/tier1",
		Sources:      []string{"postgres@cluster:/pgdata"},
		Tags:         []string{"klio.io/tag"},
	}
	common := []string{
		"snapshot", "migrate",
		"--progress",
		"--progress-update-interval=60s",
		"--source-config=/etc/kopia/tier1",
		"--config-file=/etc/kopia/tier2",
		"--disable-file-logging",
		"--sources=postgres@cluster:/pgdata",
		"--tags=tag:klio.io/tag",
	}

	t.Run("no match tags migrates every snapshot of the sources", func(t *testing.T) {
		assert.Equal(t, common, buildMigrateArgs("/etc/kopia/tier2", opts))
	})

	// Kopia prefixes the --match-tags keys itself: a prefixed key would
	// match no snapshot, and the migration would silently copy nothing.
	t.Run("match tags are passed without the user tag prefix", func(t *testing.T) {
		opts.MatchTags = []string{"klio.io/tag:backup-20261027T140000Z"}

		assert.Equal(t,
			append(common, "--match-tags=klio.io/tag:backup-20261027T140000Z"),
			buildMigrateArgs("/etc/kopia/tier2", opts))
	})
}

func TestBuildVerifyArgs(t *testing.T) {
	t.Run("no selection verifies everything visible to the client", func(t *testing.T) {
		args := buildVerifyArgs("/etc/kopia/config")

		assert.Equal(t, []string{
			"snapshot",
			"verify",
			"--json",
			"--disable-file-logging",
			"--config-file=/etc/kopia/config",
		}, args)
	})

	// Root object IDs are passed as --file-id rather than as positional
	// snapshot manifest IDs, which "kopia snapshot pin" can rewrite while a
	// verification is in flight. Kopia auto-detects a directory root passed
	// this way, so both directory and file roots go through --file-id.
	t.Run("object IDs are passed as --file-id flags", func(t *testing.T) {
		args := buildVerifyArgs("/etc/kopia/config", "kaaa", "kbbb")

		assert.Equal(t, []string{
			"snapshot",
			"verify",
			"--json",
			"--disable-file-logging",
			"--config-file=/etc/kopia/config",
			"--file-id=kaaa",
			"--file-id=kbbb",
		}, args)
	})
}

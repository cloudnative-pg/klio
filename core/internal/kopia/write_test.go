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

func TestBuildVerifyArgs(t *testing.T) {
	t.Run("no selection verifies everything visible to the client", func(t *testing.T) {
		args := buildVerifyArgs("/etc/kopia/config", VerifySnapshotsOptions{})

		assert.Equal(t, []string{
			"snapshot",
			"verify",
			"--json",
			"--disable-file-logging",
			"--config-file=/etc/kopia/config",
		}, args)
	})

	// Root object IDs are passed as --directory-id/--file-id rather than as
	// positional snapshot manifest IDs, which "kopia snapshot pin" can rewrite
	// while a verification is in flight.
	t.Run("directory IDs are passed as --directory-id flags", func(t *testing.T) {
		args := buildVerifyArgs("/etc/kopia/config", VerifySnapshotsOptions{
			DirectoryIDs: []string{"kaaa", "kbbb"},
		})

		assert.Equal(t, []string{
			"snapshot",
			"verify",
			"--json",
			"--disable-file-logging",
			"--config-file=/etc/kopia/config",
			"--directory-id=kaaa",
			"--directory-id=kbbb",
		}, args)
		assert.NotContains(t, args, "kaaa")
	})

	// A file root passed to --directory-id makes Kopia parse file content as a
	// directory listing and report a healthy object as corrupt.
	t.Run("file IDs are passed as --file-id flags", func(t *testing.T) {
		args := buildVerifyArgs("/etc/kopia/config", VerifySnapshotsOptions{
			DirectoryIDs: []string{"kaaa"},
			FileIDs:      []string{"d8fe6706"},
		})

		assert.Equal(t, []string{
			"snapshot",
			"verify",
			"--json",
			"--disable-file-logging",
			"--config-file=/etc/kopia/config",
			"--directory-id=kaaa",
			"--file-id=d8fe6706",
		}, args)
	})
}

func TestVerifySnapshotsOptions(t *testing.T) {
	t.Run("empty selection", func(t *testing.T) {
		var opts VerifySnapshotsOptions
		assert.True(t, opts.IsEmpty())
		assert.Equal(t, 0, opts.Len())
	})

	t.Run("counts both kinds of object", func(t *testing.T) {
		opts := VerifySnapshotsOptions{
			DirectoryIDs: []string{"kaaa", "kbbb"},
			FileIDs:      []string{"ccc"},
		}
		assert.False(t, opts.IsEmpty())
		assert.Equal(t, 3, opts.Len())
	})

	t.Run("file IDs alone are a selection", func(t *testing.T) {
		opts := VerifySnapshotsOptions{FileIDs: []string{"ccc"}}
		assert.False(t, opts.IsEmpty())
		assert.Equal(t, 1, opts.Len())
	})
}

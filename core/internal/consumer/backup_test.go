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

package consumer

import (
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func TestManifestListToDescriptors(t *testing.T) {
	cases := []struct {
		name     string
		entries  []kopia.Manifest
		expected []string
	}{
		{
			name:     "Empty input returns empty slice",
			entries:  []kopia.Manifest{},
			expected: []string{},
		},
		{
			name: "Single entry returns single descriptor",
			entries: []kopia.Manifest{
				{Source: kopia.SourceInfo{Host: "host", UserName: "postgres", Path: "/path"}},
			},
			expected: []string{"postgres@host:/path"},
		},
		{
			name: "Duplicate entries are de-duplicated",
			entries: []kopia.Manifest{
				{Source: kopia.SourceInfo{Host: "host", UserName: "postgres", Path: "/path"}},
				{Source: kopia.SourceInfo{Host: "host", UserName: "postgres", Path: "/path"}},
			},
			expected: []string{"postgres@host:/path"},
		},
		{
			name: "Multiple entries are sorted alphabetically",
			entries: []kopia.Manifest{
				{Source: kopia.SourceInfo{Host: "host-a", UserName: "user", Path: "/path"}},
				{Source: kopia.SourceInfo{Host: "host-b", UserName: "user", Path: "/path"}},
				{Source: kopia.SourceInfo{Host: "host-c", UserName: "user", Path: "/path"}},
			},
			expected: []string{
				"user@host-a:/path",
				"user@host-b:/path",
				"user@host-c:/path",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := manifestListToDescriptors(tc.entries)

			if !slices.Equal(got, tc.expected) {
				t.Errorf("manifestListToDescriptors() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestFindOldestWAL(t *testing.T) {
	tests := []struct {
		name     string
		backups  klioclient.BackupList
		expected string
	}{
		{
			name:     "empty backup list",
			backups:  nil,
			expected: "",
		},
		{
			name: "all backups missing StartWAL",
			backups: klioclient.BackupList{
				{StartWAL: ""},
				{StartWAL: ""},
			},
			expected: "",
		},
		{
			name: "single backup with WAL",
			backups: klioclient.BackupList{
				{StartWAL: "000000010000000000000001"},
			},
			expected: "000000010000000000000001",
		},
		{
			name: "multiple backups out of order",
			backups: klioclient.BackupList{
				{StartWAL: "00000001000000000000000F"},
				{StartWAL: "00000001000000000000000A"},
				{StartWAL: "00000001000000000000000C"},
			},
			expected: "00000001000000000000000A",
		},
		{
			name: "mix of empty and valid WALs",
			backups: klioclient.BackupList{
				{StartWAL: "000000010000000000000006"},
				{StartWAL: ""},
				{StartWAL: "000000010000000000000005"},
				{StartWAL: ""},
			},
			expected: "000000010000000000000005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findOldestWAL(tt.backups)
			if result != tt.expected {
				t.Errorf("findOldestWAL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

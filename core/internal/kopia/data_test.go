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
	"time"
)

// snapshotListSample is "kopia snapshot list --json" output for a snapshot
// created with "--tags klio.io/tag:backup-1 --tags klio.io/content:metadata"
// (trimmed to the fields Manifest decodes).
const snapshotListSample = `[
 {"id":"9ba90b3e2a5290f7d8b5062e8d37ecf3",` +
	`"source":{"host":"cluster-a","userName":"klio","path":"/var/lib/postgresql/data/pgdata"},` +
	`"description":"","startTime":"2026-10-26T10:48:10.59558Z","endTime":"2026-10-26T10:48:10.596976269Z",` +
	`"rootEntry":{"name":"pgdata","type":"d","obj":"k58b320934bf983b3063eec3afea9321e"},` +
	`"tags":{"tag:klio.io/content":"metadata","tag:klio.io/tag":"backup-1"}}
]`

func TestManifestFromSnapshotList(t *testing.T) {
	var entries []Manifest
	if err := json.Unmarshal([]byte(snapshotListSample), &entries); err != nil {
		t.Fatalf("json.Unmarshal() returned an unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("json.Unmarshal() decoded %d entries, want 1", len(entries))
	}

	m := entries[0]

	for name, want := range map[string]string{
		"klio.io/tag":     "backup-1",
		"klio.io/content": "metadata",
	} {
		if got := m.Tag(name); got != want {
			t.Errorf("Manifest.Tag(%q) = %q, want %q", name, got, want)
		}
	}

	wantStartTime := time.Date(2026, time.October, 26, 10, 48, 10, 595580000, time.UTC)
	if got := m.StartTime.ToTime(); !got.Equal(wantStartTime) {
		t.Errorf("Manifest.StartTime = %v, want %v", got, wantStartTime)
	}
}

func TestSourceInfoString(t *testing.T) {
	cases := []struct {
		name     string
		input    SourceInfo
		expected string
	}{
		{
			name:     "Global source info",
			input:    SourceInfo{},
			expected: "(global)",
		},
		{
			name:     "User and host only",
			input:    SourceInfo{UserName: "user", Host: "host"},
			expected: "user@host",
		},
		{
			name:     "Full source info",
			input:    SourceInfo{UserName: "user", Host: "host", Path: "/my/path"},
			expected: "user@host:/my/path",
		},
		{
			name:     "Path only",
			input:    SourceInfo{Path: "/only/path"},
			expected: "@:/only/path",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.String()
			if got != tc.expected {
				t.Errorf("SourceInfo.Of() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestTargetString(t *testing.T) {
	target := Target{
		Username: "user",
		Hostname: "host",
	}

	expected := "user@host"
	if got := target.String(); got != expected {
		t.Errorf("Target.Of() = %q, want %q", got, expected)
	}
}

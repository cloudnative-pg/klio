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
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func snapshot(path, backup, content, tablespace string) kopia.Manifest {
	return kopia.Manifest{
		Source: kopia.SourceInfo{Host: "c", UserName: "klio", Path: path},
		Tags: map[string]string{
			klioclient.BackupNameTagName:     backup,
			klioclient.BackupContentTagName:  content,
			klioclient.TablespaceNameTagName: tablespace,
		},
	}
}

func TestKeepUntilOnTier2(t *testing.T) {
	tier1 := []kopia.Manifest{
		snapshot("/pgdata", "b1", "pgdata", ""),
		snapshot("/pgdata_meta", "b1", "metadata", ""),
		snapshot("/tbs/a", "b1", "tablespace", "a"),
		snapshot("/pgdata", "b2", "pgdata", ""),
		snapshot("/pgdata_meta", "b2", "metadata", ""),
		snapshot("/tbs/a", "b2", "tablespace", "a"),
		snapshot("/pgdata", "b3", "pgdata", ""),
		snapshot("/pgdata_meta", "b3", "metadata", ""),
	}
	// b1 fully relayed, b2 has its metadata but not its tablespace on tier2,
	// b3 not relayed at all.
	tier2 := []kopia.Manifest{
		snapshot("/pgdata", "b1", "pgdata", ""),
		snapshot("/pgdata_meta", "b1", "metadata", ""),
		snapshot("/tbs/a", "b1", "tablespace", "a"),
		snapshot("/pgdata", "b2", "pgdata", ""),
		snapshot("/pgdata_meta", "b2", "metadata", ""),
	}

	keep := keepUntilOnTier2(tier1, tier2)
	backup := func(name string) *klioclient.BackupMetadata {
		return &klioclient.BackupMetadata{Name: name}
	}

	if keep(backup("b1")) {
		t.Error("kept b1, which is complete on tier2")
	}
	if !keep(backup("b2")) {
		t.Error("did not keep b2, whose tablespace is missing on tier2")
	}
	if !keep(backup("b3")) {
		t.Error("did not keep b3, which is not on tier2")
	}
	// A backup unknown to tier1 has nothing to protect.
	if keep(backup("b4")) {
		t.Error("kept b4, which has no tier1 snapshots")
	}
	// A backup the client never meant to relay is not waited for.
	skipped := backup("b3")
	skipped.SetAnnotation(klioclient.Tier2RelayAnnotationName, klioclient.Tier2RelaySkipped)
	if keep(skipped) {
		t.Error("kept b3, which was never meant to reach tier2")
	}
	// With no tier2 snapshots, everything on tier1 is kept.
	if keepAll := keepUntilOnTier2(tier1, nil); !keepAll(backup("b1")) {
		t.Error("keepUntilOnTier2(tier1, nil) did not keep b1")
	}
}

func TestClampWAL(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		frontier  string
		expected  string
	}{
		{
			name:      "empty frontier skips deletion",
			requested: "000000010000000000000005",
			frontier:  "",
			expected:  "",
		},
		{
			name:      "frontier older than requested clamps to frontier",
			requested: "00000001000000000000000A",
			frontier:  "000000010000000000000005",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier newer than requested keeps requested",
			requested: "000000010000000000000005",
			frontier:  "00000001000000000000000A",
			expected:  "000000010000000000000005",
		},
		{
			name:      "frontier equal to requested keeps requested",
			requested: "000000010000000000000007",
			frontier:  "000000010000000000000007",
			expected:  "000000010000000000000007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampWAL(tt.requested, tt.frontier)
			if got != tt.expected {
				t.Errorf("clampWAL(%q, %q) = %q, want %q", tt.requested, tt.frontier, got, tt.expected)
			}
		})
	}
}

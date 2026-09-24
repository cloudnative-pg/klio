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

package retention

import (
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func manifest(host, name, content, startTime string) kopia.Manifest {
	return kopia.Manifest{
		Source:    kopia.SourceInfo{Host: host},
		StartTime: startTime,
		Tags: map[string]string{
			klioclient.BackupNameTagName:    name,
			klioclient.BackupContentTagName: content,
		},
	}
}

func TestOrphanBackupNames(t *testing.T) {
	tests := []struct {
		name         string
		entries      []kopia.Manifest
		allowedHosts []string
		wantOrphans  []hostBackupName
	}{
		{
			name: "completed backup is never an orphan, even with a later backup",
			entries: []kopia.Manifest{
				manifest("cluster-a", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-a", "backup-1", "metadata", "2026-01-01T00:05:00Z"),
				manifest("cluster-a", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
				manifest("cluster-a", "backup-2", "metadata", "2026-01-02T00:05:00Z"),
			},
			allowedHosts: []string{"cluster-a"},
			wantOrphans:  nil,
		},
		{
			name: "interrupted backup with no later completed backup is left alone",
			entries: []kopia.Manifest{
				manifest("cluster-a", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-a", "backup-1", "controldata", "2026-01-01T00:01:00Z"),
			},
			allowedHosts: []string{"cluster-a"},
			wantOrphans:  nil,
		},
		{
			name: "interrupted backup with a later completed backup is an orphan",
			entries: []kopia.Manifest{
				manifest("cluster-a", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-a", "backup-1", "controldata", "2026-01-01T00:01:00Z"),
				manifest("cluster-a", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
				manifest("cluster-a", "backup-2", "metadata", "2026-01-02T00:05:00Z"),
			},
			allowedHosts: []string{"cluster-a"},
			wantOrphans:  []hostBackupName{{Host: "cluster-a", Name: "backup-1"}},
		},
		{
			name: "interrupted backup started after the latest completed backup is not an orphan",
			entries: []kopia.Manifest{
				manifest("cluster-a", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-a", "backup-1", "metadata", "2026-01-01T00:05:00Z"),
				manifest("cluster-a", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
			},
			allowedHosts: []string{"cluster-a"},
			wantOrphans:  nil,
		},
		{
			name: "host not in allowedHosts is ignored even if it looks orphaned",
			entries: []kopia.Manifest{
				manifest("cluster-b", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-b", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
				manifest("cluster-b", "backup-2", "metadata", "2026-01-02T00:05:00Z"),
			},
			allowedHosts: []string{"cluster-a"},
			wantOrphans:  nil,
		},
		{
			name: "grouping stays per-host across multiple clusters",
			entries: []kopia.Manifest{
				manifest("cluster-a", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-a", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
				manifest("cluster-a", "backup-2", "metadata", "2026-01-02T00:05:00Z"),
				manifest("cluster-b", "backup-1", "pgdata", "2026-01-01T00:00:00Z"),
				manifest("cluster-b", "backup-2", "pgdata", "2026-01-02T00:00:00Z"),
				manifest("cluster-b", "backup-2", "metadata", "2026-01-02T00:05:00Z"),
			},
			allowedHosts: []string{"cluster-a", "cluster-b"},
			wantOrphans: []hostBackupName{
				{Host: "cluster-a", Name: "backup-1"},
				{Host: "cluster-b", Name: "backup-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := orphanBackupNames(tt.entries, tt.allowedHosts)

			less := func(a, b hostBackupName) int {
				if a.Host != b.Host {
					if a.Host < b.Host {
						return -1
					}

					return 1
				}
				if a.Name < b.Name {
					return -1
				}
				if a.Name > b.Name {
					return 1
				}

				return 0
			}
			slices.SortFunc(got, less)
			slices.SortFunc(tt.wantOrphans, less)

			if !slices.Equal(got, tt.wantOrphans) {
				t.Fatalf("orphanBackupNames() = %#v, want %#v", got, tt.wantOrphans)
			}
		})
	}
}

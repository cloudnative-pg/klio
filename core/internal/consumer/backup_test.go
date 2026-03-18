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

func TestGetPinnedSnapshots(t *testing.T) {
	cases := []struct {
		name      string
		manifests []kopia.Manifest
		expected  []string
	}{
		{
			name:      "Empty input returns empty slice",
			manifests: []kopia.Manifest{},
			expected:  []string{},
		},
		{
			name: "No pinned snapshots returns empty slice",
			manifests: []kopia.Manifest{
				{ID: "snap1", RootEntry: &kopia.DirEntry{ObjID: "obj1"}},
				{ID: "snap2", RootEntry: &kopia.DirEntry{ObjID: "obj2"}},
			},
			expected: []string{},
		},
		{
			name: "Single pinned snapshot",
			manifests: []kopia.Manifest{
				{ID: "snap1", RootEntry: &kopia.DirEntry{ObjID: "obj1"}, Pins: []string{"pin1"}},
				{ID: "snap2", RootEntry: &kopia.DirEntry{ObjID: "obj2"}},
			},
			expected: []string{"obj1"},
		},
		{
			name: "Multiple pinned snapshots are sorted",
			manifests: []kopia.Manifest{
				{ID: "snap-c", RootEntry: &kopia.DirEntry{ObjID: "obj-c"}, Pins: []string{"pin1"}},
				{ID: "snap-a", RootEntry: &kopia.DirEntry{ObjID: "obj-a"}, Pins: []string{"pin2"}},
				{ID: "snap-b", RootEntry: &kopia.DirEntry{ObjID: "obj-b"}},
			},
			expected: []string{"obj-a", "obj-c"},
		},
		{
			name: "Duplicate ObjIDs are de-duplicated",
			manifests: []kopia.Manifest{
				{ID: "snap1", RootEntry: &kopia.DirEntry{ObjID: "obj1"}, Pins: []string{"pin1"}},
				{ID: "snap2", RootEntry: &kopia.DirEntry{ObjID: "obj1"}, Pins: []string{"pin2"}},
			},
			expected: []string{"obj1"},
		},
		{
			name: "Nil RootEntry with pins is skipped",
			manifests: []kopia.Manifest{
				{ID: "snap1", RootEntry: nil, Pins: []string{"pin1"}},
				{ID: "snap2", RootEntry: &kopia.DirEntry{ObjID: "obj2"}, Pins: []string{"pin2"}},
			},
			expected: []string{"obj2"},
		},
		{
			name: "Empty ObjID with pins is skipped",
			manifests: []kopia.Manifest{
				{ID: "snap1", RootEntry: &kopia.DirEntry{ObjID: ""}, Pins: []string{"pin1"}},
				{ID: "snap2", RootEntry: &kopia.DirEntry{ObjID: "obj2"}, Pins: []string{"pin2"}},
			},
			expected: []string{"obj2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getPinnedSnapshots(tc.manifests)

			if !slices.Equal(got, tc.expected) {
				t.Errorf("getPinnedSnapshots() = %v, want %v", got, tc.expected)
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

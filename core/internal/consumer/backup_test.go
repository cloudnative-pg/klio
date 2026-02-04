package consumer

import (
	"slices"
	"testing"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/kopia"
)

func TestSourceInfoListToDescriptors(t *testing.T) {
	cases := []struct {
		name     string
		entries  []kopia.SourceInfo
		expected []string
	}{
		{
			name:     "Empty input returns empty slice",
			entries:  []kopia.SourceInfo{},
			expected: []string{},
		},
		{
			name: "Single entry returns single descriptor",
			entries: []kopia.SourceInfo{
				{Host: "host", UserName: "postgres", Path: "/path"},
			},
			expected: []string{"postgres@host:/path"},
		},
		{
			name: "Duplicate entries are de-duplicated",
			entries: []kopia.SourceInfo{
				{Host: "host", UserName: "postgres", Path: "/path"},
				{Host: "host", UserName: "postgres", Path: "/path"},
			},
			expected: []string{"postgres@host:/path"},
		},
		{
			name: "Multiple entries are sorted alphabetically",
			entries: []kopia.SourceInfo{
				{Host: "host-a", UserName: "user", Path: "/path"},
				{Host: "host-b", UserName: "user", Path: "/path"},
				{Host: "host-c", UserName: "user", Path: "/path"},
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
			got := sourceInfoListToDescriptors(tc.entries)

			if !slices.Equal(got, tc.expected) {
				t.Errorf("sourceInfoListToDescriptors() = %v, want %v", got, tc.expected)
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

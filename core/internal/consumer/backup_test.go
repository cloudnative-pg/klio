package consumer

import (
	"slices"
	"testing"

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

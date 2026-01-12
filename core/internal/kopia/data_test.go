package kopia

import (
	"testing"
)

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
				t.Errorf("SourceInfo.String() = %q, want %q", got, tc.expected)
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
		t.Errorf("Target.String() = %q, want %q", got, expected)
	}
}

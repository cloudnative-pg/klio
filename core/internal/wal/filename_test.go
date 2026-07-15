package wal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsPartialWALFile verifies that only the .partial variant of a valid WAL
// segment is recognized as partial: bare segments, history and backup-label
// files, and malformed .partial names are not.
func TestIsPartialWALFile(t *testing.T) {
	tests := map[string]bool{
		"000000010000000000000001.partial":         true,
		"000000010000000000000001":                 false,
		"00000002.history":                         false,
		"000000010000000000000001.00000028.backup": false,
		"garbage.partial":                          false,
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, IsPartialWALFile(name))
		})
	}
}

func TestIsWALSegmentOrPartial(t *testing.T) {
	tests := map[string]bool{
		"000000010000000000000001":                 true,
		"000000010000000000000001.partial":         true,
		"00000002.history":                         false,
		"000000010000000000000001.00000028.backup": false,
		"garbage.partial":                          false,
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, IsWALSegmentOrPartial(name))
		})
	}
}

func TestTrimPartialSuffix(t *testing.T) {
	tests := map[string]string{
		"000000010000000000000001.partial": "000000010000000000000001",
		"000000010000000000000001":         "000000010000000000000001",
		"00000002.history":                 "00000002.history",
		"garbage.partial":                  "garbage",
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, TrimPartialSuffix(name))
		})
	}
}

func TestWithPartialSuffix(t *testing.T) {
	tests := map[string]string{
		"000000010000000000000001.partial": "000000010000000000000001.partial",
		"000000010000000000000001":         "000000010000000000000001.partial",
		"garbage.partial":                  "garbage.partial",
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, WithPartialSuffix(name))
		})
	}
}

package kopiaserver

import (
	"testing"
)

func TestIsACLsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "ACLs already enabled message",
			output:   "Error: ACLs already enabled",
			expected: true,
		},
		{
			name:     "ACLs already enabled in longer output",
			output:   "Some other text\nACLs already enabled\nMore text",
			expected: true,
		},
		{
			name:     "ACLs not enabled",
			output:   "ACLs have been enabled successfully",
			expected: false,
		},
		{
			name:     "Empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "Different error message",
			output:   "Error: something else went wrong",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isACLsEnabled(tt.output)
			if result != tt.expected {
				t.Errorf("isACLsEnabled(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}

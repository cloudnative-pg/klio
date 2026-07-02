package consumer

import "testing"

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

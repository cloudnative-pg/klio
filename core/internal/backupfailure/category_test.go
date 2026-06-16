package backupfailure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByExitCode(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		want   Category
		wantOK bool
	}{
		{"repository", RepositoryError.ExitCode, RepositoryError, true},
		{"source", SourceError.ExitCode, SourceError, true},
		{"verification", Verification.ExitCode, Verification, true},
		{"default cobra exit", 1, Category{}, false},
		{"unrecognized code", 99, Category{}, false},
		{"signal kill", -1, Category{}, false},
		{"zero is never a category", 0, Category{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ByExitCode(tc.code)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestCategoriesAreWellFormed guards the single-source-of-truth
// invariants: every category has a name, and the subprocess-reported
// exit codes are unique. A duplicate or missing value would silently
// misclassify failures.
func TestCategoriesAreWellFormed(t *testing.T) {
	seenNames := make(map[string]bool)
	seenCodes := make(map[int]bool)

	for _, c := range categories() {
		assert.NotEmpty(t, c.Name, "category has no name")

		assert.False(t, seenNames[c.Name], "duplicate category name %q", c.Name)
		seenNames[c.Name] = true

		if c.ExitCode == 0 {
			continue
		}
		assert.False(t, seenCodes[c.ExitCode], "duplicate exit code %d", c.ExitCode)
		seenCodes[c.ExitCode] = true
	}
}

func TestNames(t *testing.T) {
	names := Names()
	assert.Len(t, names, len(categories()))
	assert.Equal(t, RepositoryError.Name, names[0])
}

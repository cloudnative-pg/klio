package kopia

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVerifyOutput(t *testing.T) {
	t.Run("valid JSON with errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":2,"errorStrings":["bad object 1","bad object 2"]}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 2, result.ErrorCount)
		assert.Equal(t, []string{"bad object 1", "bad object 2"}, result.ErrorStrings)
	})

	t.Run("valid JSON with no errors", func(t *testing.T) {
		stdout := []byte(`{"errorCount":0}`)
		result := parseVerifyOutput(stdout)
		assert.Equal(t, 0, result.ErrorCount)
		assert.Empty(t, result.ErrorStrings)
	})

	t.Run("empty output returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte{})
		assert.Equal(t, VerifyResult{}, result)
	})

	t.Run("invalid JSON returns zero result", func(t *testing.T) {
		result := parseVerifyOutput([]byte("not json"))
		assert.Equal(t, VerifyResult{}, result)
	})
}

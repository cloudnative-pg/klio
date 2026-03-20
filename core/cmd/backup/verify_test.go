package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTiers(t *testing.T) {
	t.Run("both tiers", func(t *testing.T) {
		result := parseTiers("tier1,tier2")
		assert.True(t, result.Tier1)
		assert.True(t, result.Tier2)
	})

	t.Run("tier1 only", func(t *testing.T) {
		result := parseTiers("tier1")
		assert.True(t, result.Tier1)
		assert.False(t, result.Tier2)
	})

	t.Run("tier2 only", func(t *testing.T) {
		result := parseTiers("tier2")
		assert.False(t, result.Tier1)
		assert.True(t, result.Tier2)
	})

	t.Run("reversed order", func(t *testing.T) {
		result := parseTiers("tier2,tier1")
		assert.True(t, result.Tier1)
		assert.True(t, result.Tier2)
	})

	t.Run("with spaces", func(t *testing.T) {
		result := parseTiers("tier1 , tier2")
		assert.True(t, result.Tier1)
		assert.True(t, result.Tier2)
	})

	t.Run("empty string returns no tiers", func(t *testing.T) {
		result := parseTiers("")
		assert.False(t, result.Tier1)
		assert.False(t, result.Tier2)
	})

	t.Run("invalid tier names are ignored", func(t *testing.T) {
		result := parseTiers("tier1,invalid,tier3")
		assert.True(t, result.Tier1)
		assert.False(t, result.Tier2)
	})

	t.Run("duplicate tiers", func(t *testing.T) {
		result := parseTiers("tier1,tier1,tier2")
		assert.True(t, result.Tier1)
		assert.True(t, result.Tier2)
	})
}

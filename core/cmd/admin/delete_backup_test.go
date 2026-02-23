package admin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	klioGRPC "github.com/cloudnative-pg/klio/core/internal/grpc"
)

func TestBuildTiersList(t *testing.T) {
	tests := []struct {
		name     string
		tier1    bool
		tier2    bool
		expected []klioGRPC.Tier
	}{
		{
			name:     "no tiers",
			tier1:    false,
			tier2:    false,
			expected: nil,
		},
		{
			name:     "tier1 only",
			tier1:    true,
			tier2:    false,
			expected: []klioGRPC.Tier{klioGRPC.Tier_TIER_1},
		},
		{
			name:     "tier2 only",
			tier1:    false,
			tier2:    true,
			expected: []klioGRPC.Tier{klioGRPC.Tier_TIER_2},
		},
		{
			name:     "both tiers",
			tier1:    true,
			tier2:    true,
			expected: []klioGRPC.Tier{klioGRPC.Tier_TIER_1, klioGRPC.Tier_TIER_2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTiersList(tt.tier1, tt.tier2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

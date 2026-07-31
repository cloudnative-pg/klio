/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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

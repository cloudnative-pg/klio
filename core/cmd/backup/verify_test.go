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

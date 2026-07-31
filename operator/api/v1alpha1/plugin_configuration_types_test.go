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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWALPrefetch(t *testing.T) {
	t.Run("nil WALPrefetch returns defaults", func(t *testing.T) {
		spec := &PluginConfigurationSpec{
			WALPrefetch: nil,
		}

		result := spec.GetWALPrefetch()

		assert.Equal(t, 2, result.Count)
		assert.Equal(t, 4, result.MaxConcurrentDownloads)
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		spec := &PluginConfigurationSpec{
			WALPrefetch: &WALPrefetchConfiguration{
				Count:                  8,
				MaxConcurrentDownloads: 16,
			},
		}

		result := spec.GetWALPrefetch()

		assert.Equal(t, 8, result.Count)
		assert.Equal(t, 16, result.MaxConcurrentDownloads)
	})

	t.Run("count zero disables prefetching", func(t *testing.T) {
		spec := &PluginConfigurationSpec{
			WALPrefetch: &WALPrefetchConfiguration{
				Count:                  0,
				MaxConcurrentDownloads: 4,
			},
		}

		result := spec.GetWALPrefetch()

		assert.Equal(t, 0, result.Count)
		assert.Equal(t, 4, result.MaxConcurrentDownloads)
	})

	t.Run("zero MaxConcurrentDownloads gets defaulted", func(t *testing.T) {
		spec := &PluginConfigurationSpec{
			WALPrefetch: &WALPrefetchConfiguration{
				Count:                  3,
				MaxConcurrentDownloads: 0,
			},
		}

		result := spec.GetWALPrefetch()

		assert.Equal(t, 3, result.Count)
		assert.Equal(t, 4, result.MaxConcurrentDownloads)
	})
}

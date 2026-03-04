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

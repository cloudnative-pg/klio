package cnpgi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

func TestConvertRetentionPolicy(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := convertRetentionPolicy(nil)
		assert.Nil(t, result)
	})

	t.Run("all fields set", func(t *testing.T) {
		input := &kliov1alpha1.RetentionPolicy{
			KeepLatest:  ptr.To(5),
			KeepAnnual:  ptr.To(2),
			KeepMonthly: ptr.To(6),
			KeepWeekly:  ptr.To(4),
			KeepDaily:   ptr.To(7),
			KeepHourly:  ptr.To(24),
		}

		result := convertRetentionPolicy(input)

		assert.NotNil(t, result)
		assert.Equal(t, &config.RetentionPolicy{
			KeepLatest:  ptr.To(5),
			KeepAnnual:  ptr.To(2),
			KeepMonthly: ptr.To(6),
			KeepWeekly:  ptr.To(4),
			KeepDaily:   ptr.To(7),
			KeepHourly:  ptr.To(24),
		}, result)
	})

	t.Run("partial fields set", func(t *testing.T) {
		input := &kliov1alpha1.RetentionPolicy{
			KeepLatest: ptr.To(3),
			KeepDaily:  ptr.To(7),
		}

		result := convertRetentionPolicy(input)

		assert.NotNil(t, result)
		assert.Equal(t, ptr.To(3), result.KeepLatest)
		assert.Equal(t, ptr.To(7), result.KeepDaily)
		assert.Nil(t, result.KeepAnnual)
		assert.Nil(t, result.KeepMonthly)
		assert.Nil(t, result.KeepWeekly)
		assert.Nil(t, result.KeepHourly)
	})
}

func TestConvertTier1RetentionPolicy(t *testing.T) {
	t.Run("nil tier1 returns nil", func(t *testing.T) {
		result := convertTier1RetentionPolicy(nil)
		assert.Nil(t, result)
	})

	t.Run("tier1 with nil retention returns nil", func(t *testing.T) {
		tier1 := &kliov1alpha1.Tier1PluginConfiguration{}
		result := convertTier1RetentionPolicy(tier1)
		assert.Nil(t, result)
	})

	t.Run("tier1 with retention policy", func(t *testing.T) {
		tier1 := &kliov1alpha1.Tier1PluginConfiguration{
			RetentionPolicy: &kliov1alpha1.RetentionPolicy{
				KeepLatest: ptr.To(10),
			},
		}

		result := convertTier1RetentionPolicy(tier1)

		assert.NotNil(t, result)
		assert.Equal(t, ptr.To(10), result.KeepLatest)
	})
}

func TestConvertTier2RetentionPolicy(t *testing.T) {
	t.Run("nil tier2 returns nil", func(t *testing.T) {
		result := convertTier2RetentionPolicy(nil)
		assert.Nil(t, result)
	})

	t.Run("tier2 with nil retention returns nil", func(t *testing.T) {
		tier2 := &kliov1alpha1.Tier2PluginConfiguration{
			EnableBackup: true,
		}
		result := convertTier2RetentionPolicy(tier2)
		assert.Nil(t, result)
	})

	t.Run("tier2 with retention policy", func(t *testing.T) {
		tier2 := &kliov1alpha1.Tier2PluginConfiguration{
			EnableBackup: true,
			RetentionPolicy: &kliov1alpha1.RetentionPolicy{
				KeepDaily:  ptr.To(7),
				KeepWeekly: ptr.To(4),
			},
		}

		result := convertTier2RetentionPolicy(tier2)

		assert.NotNil(t, result)
		assert.Equal(t, ptr.To(7), result.KeepDaily)
		assert.Equal(t, ptr.To(4), result.KeepWeekly)
	})
}

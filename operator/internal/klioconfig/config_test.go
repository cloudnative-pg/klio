package klioconfig

import (
	"errors"
	"fmt"
	"net"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

const (
	testServerAddress = "klio.example"
	testClusterName   = "my-cluster"
	testCustomKey     = "custom-key"
)

//nolint:maintidx // table-driven test with many cases
func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name       string
		spec       kliov1alpha1.PluginConfigurationSpec
		configKey  string
		assertions func(t *testing.T, cfg *config.Data)
	}{
		{
			name: "standard mode enables tier1 and populates tier1 URLs",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.True(t, cfg.Tier1Enabled)
				assert.Equal(t,
					"https://"+net.JoinHostPort(testServerAddress, KlioTier1HTTPPort),
					cfg.Client.Base.URL)
				assert.Equal(t,
					net.JoinHostPort(testServerAddress, KlioTier1GRPCPort),
					cfg.Client.Wal.Address)
			},
		},
		{
			name: "read-only mode disables tier1 and leaves tier1 URLs empty",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeReadOnly,
				ClusterName:   testClusterName,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.False(t, cfg.Tier1Enabled)
				assert.Empty(t, cfg.Client.Base.URL)
				assert.Empty(t, cfg.Client.Wal.Address)
			},
		},
		{
			name: "tier2 backup enabled populates tier2 URLs",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				Tier2: &kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup: true,
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.True(t, cfg.Tier2BackupEnabled)
				assert.False(t, cfg.Tier2RecoveryEnabled)
				assert.Equal(t,
					"https://"+net.JoinHostPort(testServerAddress, KlioTier2HTTPPort),
					cfg.Client.Base.Tier2URL)
				assert.Equal(t,
					net.JoinHostPort(testServerAddress, KlioTier2GRPCPort),
					cfg.Client.Wal.Tier2Address)
			},
		},
		{
			name: "tier2 recovery enabled populates tier2 URLs",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ClusterName:   testClusterName,
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeReadOnly,
				Tier2: &kliov1alpha1.Tier2PluginConfiguration{
					EnableRecovery: true,
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.True(t, cfg.Tier2RecoveryEnabled)
				assert.False(t, cfg.Tier2BackupEnabled)
				assert.NotEmpty(t, cfg.Client.Base.Tier2URL)
				assert.NotEmpty(t, cfg.Client.Wal.Tier2Address)
			},
		},
		{
			name: "tier2 disabled leaves tier2 URLs empty",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ClusterName:   testClusterName,
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.False(t, cfg.Tier2BackupEnabled)
				assert.False(t, cfg.Tier2RecoveryEnabled)
				assert.Empty(t, cfg.Client.Base.Tier2URL)
				assert.Empty(t, cfg.Client.Wal.Tier2Address)
			},
		},
		{
			name: "ClusterName from spec is used",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.Equal(t, testClusterName, cfg.Client.ClusterName)
			},
		},
		{
			name: "TLS cert paths derived from configKey",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
			},
			configKey: testCustomKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				serverPath := GetServerSecretVolumeMountPath(testCustomKey)
				clientPath := GetClientSecretVolumeMountPath(testCustomKey)
				assert.Equal(t, path.Join(serverPath, "tls.crt"), cfg.Client.Base.ServerCertPath)
				assert.Equal(t, path.Join(clientPath, "tls.crt"), cfg.Client.Base.ClientCertPath)
				assert.Equal(t, path.Join(clientPath, "tls.key"), cfg.Client.Base.ClientKeyPath)
			},
		},
		{
			name: "WALPrefetch configuration with custom values",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				WALPrefetch: &kliov1alpha1.WALPrefetchConfiguration{
					Count:                  8,
					MaxConcurrentDownloads: 16,
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.Equal(t, 8, cfg.WALPrefetch.Count)
				assert.Equal(t, 16, cfg.WALPrefetch.MaxConcurrentDownloads)
			},
		},
		{
			name: "WALPrefetch defaults when not specified",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				// Default values from GetWALPrefetch
				assert.Equal(t, 2, cfg.WALPrefetch.Count)
				assert.Equal(t, 4, cfg.WALPrefetch.MaxConcurrentDownloads)
			},
		},
		{
			name: "WALPrefetch with zero count (disabled)",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				WALPrefetch: &kliov1alpha1.WALPrefetchConfiguration{
					Count:                  0,
					MaxConcurrentDownloads: 4,
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.Equal(t, 0, cfg.WALPrefetch.Count) // Zero is valid (disables prefetching)
				assert.Equal(t, 4, cfg.WALPrefetch.MaxConcurrentDownloads)
			},
		},
		{
			name: "tier1 retention policy is set",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				Tier1: &kliov1alpha1.Tier1PluginConfiguration{
					RetentionPolicy: &kliov1alpha1.RetentionPolicy{
						KeepLatest: new(5),
						KeepDaily:  new(7),
					},
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.NotNil(t, cfg.Tier1RetentionPolicy)
				assert.Equal(t, new(5), cfg.Tier1RetentionPolicy.KeepLatest)
				assert.Equal(t, new(7), cfg.Tier1RetentionPolicy.KeepDaily)
			},
		},
		{
			name: "tier2 retention policy is set",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				Tier2: &kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup: true,
					RetentionPolicy: &kliov1alpha1.RetentionPolicy{
						KeepWeekly:  new(4),
						KeepMonthly: new(12),
					},
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.NotNil(t, cfg.Tier2RetentionPolicy)
				assert.Equal(t, new(4), cfg.Tier2RetentionPolicy.KeepWeekly)
				assert.Equal(t, new(12), cfg.Tier2RetentionPolicy.KeepMonthly)
			},
		},
		{
			name: "both tier1 and tier2 retention policies",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
				Tier1: &kliov1alpha1.Tier1PluginConfiguration{
					RetentionPolicy: &kliov1alpha1.RetentionPolicy{
						KeepLatest: new(3),
					},
				},
				Tier2: &kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup: true,
					RetentionPolicy: &kliov1alpha1.RetentionPolicy{
						KeepLatest: new(10),
					},
				},
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.NotNil(t, cfg.Tier1RetentionPolicy)
				assert.NotNil(t, cfg.Tier2RetentionPolicy)
				assert.Equal(t, new(3), cfg.Tier1RetentionPolicy.KeepLatest)
				assert.Equal(t, new(10), cfg.Tier2RetentionPolicy.KeepLatest)
			},
		},
		{
			name: "source config has default values",
			spec: kliov1alpha1.PluginConfigurationSpec{
				ServerAddress: testServerAddress,
				Mode:          kliov1alpha1.ModeStandard,
				ClusterName:   testClusterName,
			},
			configKey: ArchiveConfigKey,
			assertions: func(t *testing.T, cfg *config.Data) {
				t.Helper()
				assert.Equal(t, "user=postgres replication=yes application_name=klio", cfg.Source.DSN)
				assert.Equal(t, "user=postgres application_name=klio", cfg.Source.StandardDSN)
				assert.Equal(t, "klio", cfg.Source.Slot)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateConfig(tc.spec, tc.configKey)
			tc.assertions(t, result)
		})
	}
}

func TestConvertRetentionPolicy(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := convertRetentionPolicy(nil)
		assert.Nil(t, result)
	})

	t.Run("all fields set", func(t *testing.T) {
		input := &kliov1alpha1.RetentionPolicy{
			KeepLatest:  new(5),
			KeepAnnual:  new(2),
			KeepMonthly: new(6),
			KeepWeekly:  new(4),
			KeepDaily:   new(7),
			KeepHourly:  new(24),
		}

		result := convertRetentionPolicy(input)

		assert.NotNil(t, result)
		assert.Equal(t, &config.RetentionPolicy{
			KeepLatest:  new(5),
			KeepAnnual:  new(2),
			KeepMonthly: new(6),
			KeepWeekly:  new(4),
			KeepDaily:   new(7),
			KeepHourly:  new(24),
		}, result)
	})

	t.Run("partial fields set", func(t *testing.T) {
		input := &kliov1alpha1.RetentionPolicy{
			KeepLatest: new(3),
			KeepDaily:  new(7),
		}

		result := convertRetentionPolicy(input)

		assert.NotNil(t, result)
		assert.Equal(t, new(3), result.KeepLatest)
		assert.Equal(t, new(7), result.KeepDaily)
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
				KeepLatest: new(10),
			},
		}

		result := convertTier1RetentionPolicy(tier1)

		assert.NotNil(t, result)
		assert.Equal(t, new(10), result.KeepLatest)
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
				KeepDaily:  new(7),
				KeepWeekly: new(4),
			},
		}

		result := convertTier2RetentionPolicy(tier2)

		assert.NotNil(t, result)
		assert.Equal(t, new(7), result.KeepDaily)
		assert.Equal(t, new(4), result.KeepWeekly)
	})
}

func TestPluginConfigurationNotFoundError(t *testing.T) {
	t.Run("error message includes name", func(t *testing.T) {
		err := &PluginConfigurationNotFoundError{Name: "my-config"}
		assert.Equal(t, `PluginConfiguration "my-config" not found`, err.Error())
	})

	t.Run("error message with empty name", func(t *testing.T) {
		err := &PluginConfigurationNotFoundError{Name: ""}
		assert.Equal(t, `PluginConfiguration "" not found`, err.Error())
	})
}

func TestIsPluginConfigurationNotFound(t *testing.T) {
	t.Run("returns false for nil error", func(t *testing.T) {
		result := IsPluginConfigurationNotFound(nil)
		assert.False(t, result)
	})

	t.Run("returns true for direct PluginConfigurationNotFoundError", func(t *testing.T) {
		err := &PluginConfigurationNotFoundError{Name: "test-config"}
		result := IsPluginConfigurationNotFound(err)
		assert.True(t, result)
	})

	t.Run("returns true for wrapped PluginConfigurationNotFoundError", func(t *testing.T) {
		innerErr := &PluginConfigurationNotFoundError{Name: "test-config"}
		wrappedErr := fmt.Errorf("context: %w", innerErr)
		result := IsPluginConfigurationNotFound(wrappedErr)
		assert.True(t, result)
	})

	t.Run("returns true for deeply wrapped PluginConfigurationNotFoundError", func(t *testing.T) {
		innerErr := &PluginConfigurationNotFoundError{Name: "test-config"}
		wrappedOnce := fmt.Errorf("level 1: %w", innerErr)
		wrappedTwice := fmt.Errorf("level 2: %w", wrappedOnce)
		result := IsPluginConfigurationNotFound(wrappedTwice)
		assert.True(t, result)
	})

	t.Run("returns false for regular error", func(t *testing.T) {
		err := errors.New("some other error")
		result := IsPluginConfigurationNotFound(err)
		assert.False(t, result)
	})

	t.Run("returns false for wrapped regular error", func(t *testing.T) {
		innerErr := errors.New("base error")
		wrappedErr := fmt.Errorf("context: %w", innerErr)
		result := IsPluginConfigurationNotFound(wrappedErr)
		assert.False(t, result)
	})
}

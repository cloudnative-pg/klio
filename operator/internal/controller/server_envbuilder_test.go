package controller

import (
	"testing"

	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

func findEnvVar(envVars []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envVars {
		if envVars[i].Name == name {
			return &envVars[i]
		}
	}

	return nil
}

func TestGetCoreEnvVarsIncludesQueueWhenTier1Configured(t *testing.T) {
	builder := &envBuilder{
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKey: &machineryapi.SecretKeySelector{
				LocalObjectReference: machineryapi.LocalObjectReference{Name: "secret"},
				Key:                  "key",
			},
		},
	}

	envVars := builder.getCoreEnvVars()
	queueDir := findEnvVar(envVars, "QUEUE_DIRECTORY")
	require.NotNil(t, queueDir)
	assert.Equal(t, "/queue", queueDir.Value)
}

func TestGetCoreEnvVarsExcludesQueueWhenNoTier1(t *testing.T) {
	builder := &envBuilder{
		tier1: nil,
	}

	envVars := builder.getCoreEnvVars()
	queueDir := findEnvVar(envVars, "QUEUE_DIRECTORY")
	assert.Nil(t, queueDir)
}

func TestGetCoreEnvVarsIncludesTier1EnvVars(t *testing.T) {
	builder := &envBuilder{
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKey: &machineryapi.SecretKeySelector{
				LocalObjectReference: machineryapi.LocalObjectReference{Name: "enc-secret"},
				Key:                  "encryptionKey",
			},
		},
	}

	envVars := builder.getCoreEnvVars()

	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_CACHE"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_REPOSITORY"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_LISTEN_ADDRESS"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_WAL_LISTEN_ADDRESS"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_WAL_PATH"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_ENCRYPTION_KEY"))
}

func TestGetCoreEnvVarsOnlyTLSWhenNoTier1(t *testing.T) {
	builder := &envBuilder{
		tier1: nil,
	}

	envVars := builder.getCoreEnvVars()
	assert.Len(t, envVars, 3)
	assert.NotNil(t, findEnvVar(envVars, "TLS_CERT"))
	assert.NotNil(t, findEnvVar(envVars, "TLS_KEY"))
	assert.NotNil(t, findEnvVar(envVars, "TLS_CLIENT_CA_CERT"))
}

func TestGetTier2EnvVarsExcludesQueueDirectory(t *testing.T) {
	builder := &envBuilder{
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKey: &machineryapi.SecretKeySelector{
				LocalObjectReference: machineryapi.LocalObjectReference{Name: "secret"},
				Key:                  "key",
			},
		},
		tier2: &kliov1alpha1.Tier2Configuration{
			S3: &kliov1alpha1.S3Configuration{
				BucketName: "test-bucket",
			},
			EncryptionKey: &machineryapi.SecretKeySelector{
				LocalObjectReference: machineryapi.LocalObjectReference{Name: "secret"},
				Key:                  "key",
			},
		},
	}

	envVars := builder.getTier2EnvVars()
	queueDir := findEnvVar(envVars, "QUEUE_DIRECTORY")
	assert.Nil(t, queueDir)
}

func TestGetTier2EnvVarsNilWhenNoTier2(t *testing.T) {
	builder := &envBuilder{
		tier2: nil,
	}

	envVars := builder.getTier2EnvVars()
	assert.Nil(t, envVars)
}

func TestQueueDirectoryAppearsOnceWithBothTiers(t *testing.T) {
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKey: &machineryapi.SecretKeySelector{
					LocalObjectReference: machineryapi.LocalObjectReference{Name: "secret"},
					Key:                  "key",
				},
			},
			Tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName: "test-bucket",
				},
				EncryptionKey: &machineryapi.SecretKeySelector{
					LocalObjectReference: machineryapi.LocalObjectReference{Name: "secret"},
					Key:                  "key",
				},
			},
		},
	}

	envVars := newServerEnvBuilder(server).addCommonEnvs().build()

	var count int
	for _, env := range envVars {
		if env.Name == "QUEUE_DIRECTORY" {
			count++
		}
	}

	assert.Equal(t, 1, count, "QUEUE_DIRECTORY should appear exactly once")
}

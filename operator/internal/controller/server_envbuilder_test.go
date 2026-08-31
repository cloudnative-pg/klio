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

package controller

import (
	"testing"

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

func newTestFileSource(secretName, filePath string) kliov1alpha1.FileSource {
	return kliov1alpha1.FileSource{
		FileReference: &kliov1alpha1.FileReference{
			Volume: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
			Path: filePath,
		},
	}
}

func TestGetCoreEnvVarsIncludesQueueWhenTier1Configured(t *testing.T) {
	builder := &envBuilder{
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
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
			EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
		},
	}

	envVars := builder.getCoreEnvVars()

	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_CACHE"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_REPOSITORY"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_LISTEN_ADDRESS"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_WAL_LISTEN_ADDRESS"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_WAL_PATH"))

	encKeyFile := findEnvVar(envVars, "TIER1_ENCRYPTION_KEY_FILE")
	require.NotNil(t, encKeyFile)
	assert.Equal(t, "/files/tier1-enc-key-file/encryption-key.age", encKeyFile.Value)

	identityFile := findEnvVar(envVars, "TIER1_IDENTITY_FILE")
	require.NotNil(t, identityFile)
	assert.Equal(t, "/files/tier1-identity/identity.txt", identityFile.Value)
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
			EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
		},
		tier2: &kliov1alpha1.Tier2Configuration{
			S3: &kliov1alpha1.S3Configuration{
				BucketName: "test-bucket",
			},
			EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
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
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
			Tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName: "test-bucket",
				},
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
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

func TestGetTier2EnvVars(t *testing.T) {
	builder := &envBuilder{
		tier2: &kliov1alpha1.Tier2Configuration{
			S3: &kliov1alpha1.S3Configuration{
				BucketName: "test-bucket",
			},
			EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
		},
	}

	envVars := builder.getTier2EnvVars()

	encKeyFile := findEnvVar(envVars, "TIER2_ENCRYPTION_KEY_FILE")
	require.NotNil(t, encKeyFile)
	assert.Equal(t, "/files/tier2-enc-key-file/encryption-key.age", encKeyFile.Value)

	identityFile := findEnvVar(envVars, "TIER2_IDENTITY_FILE")
	require.NotNil(t, identityFile)
	assert.Equal(t, "/files/tier2-identity/identity.txt", identityFile.Value)
}

func TestBuildVolumes(t *testing.T) {
	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "ca-secret",
			},
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
		},
	}

	volumes := r.buildVolumes(server)

	findVolume := func(name string) *corev1.Volume {
		for i := range volumes {
			if volumes[i].Name == name {
				return &volumes[i]
			}
		}

		return nil
	}

	encVol := findVolume(tier1EncKeyFileVolName)
	require.NotNil(t, encVol)
	assert.Equal(t, "enc-secret", encVol.Secret.SecretName)

	idVol := findVolume(tier1IdentityVolName)
	require.NotNil(t, idVol)
	assert.Equal(t, "id-secret", idVol.Secret.SecretName)
}

func TestBuildVolumeMounts(t *testing.T) {
	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
		},
	}

	mounts := r.buildVolumeMounts(server)

	findMount := func(name string) *corev1.VolumeMount {
		for i := range mounts {
			if mounts[i].Name == name {
				return &mounts[i]
			}
		}

		return nil
	}

	encMount := findMount(tier1EncKeyFileVolName)
	require.NotNil(t, encMount)
	assert.Equal(t, "/files/tier1-enc-key-file", encMount.MountPath)
	assert.True(t, encMount.ReadOnly)

	idMount := findMount(tier1IdentityVolName)
	require.NotNil(t, idMount)
	assert.Equal(t, "/files/tier1-identity", idMount.MountPath)
	assert.True(t, idMount.ReadOnly)
}

func TestBuildIdentityVolumeDefaultMode(t *testing.T) {
	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			TLSConfiguration: kliov1alpha1.TLSConfiguration{
				TLSSecretName:      "tls-secret",
				ClientCASecretName: "ca-secret",
			},
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
			},
		},
	}

	volumes := r.buildVolumes(server)

	findVolume := func(name string) *corev1.Volume {
		for i := range volumes {
			if volumes[i].Name == name {
				return &volumes[i]
			}
		}

		return nil
	}

	// Encryption key volume should NOT have DefaultMode forced.
	encVol := findVolume(tier1EncKeyFileVolName)
	require.NotNil(t, encVol)
	assert.Nil(t, encVol.Secret.DefaultMode)

	// Identity volume MUST have DefaultMode 0400.
	idVol := findVolume(tier1IdentityVolName)
	require.NotNil(t, idVol)
	require.NotNil(t, idVol.Secret.DefaultMode)
	assert.Equal(t, int32(0o400), *idVol.Secret.DefaultMode)
}

func TestBuildIdentityVolMountConfigMap(t *testing.T) {
	vol, mount := buildIdentityVolMount("test-id", kliov1alpha1.FileSource{
		FileReference: &kliov1alpha1.FileReference{
			Volume: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cm"},
				},
			},
			Path: "identity.txt",
		},
	})

	require.NotNil(t, vol.ConfigMap.DefaultMode)
	assert.Equal(t, int32(0o400), *vol.ConfigMap.DefaultMode)
	assert.True(t, mount.ReadOnly)
}

func TestBuildIdentityVolMountProjected(t *testing.T) {
	vol, mount := buildIdentityVolMount("test-id", kliov1alpha1.FileSource{
		FileReference: &kliov1alpha1.FileReference{
			Volume: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{},
			},
			Path: "identity.txt",
		},
	})

	require.NotNil(t, vol.Projected.DefaultMode)
	assert.Equal(t, int32(0o400), *vol.Projected.DefaultMode)
	assert.True(t, mount.ReadOnly)
}

func TestCacheEnvVars(t *testing.T) {
	tests := map[string]struct {
		dedicated bool
		// The cache in use, and the location to reclaim because the cache
		// moved away from it.
		tier1, tier1Stale string
		tier2, tier2Stale string
	}{
		"dedicated volumes": {
			dedicated:  true,
			tier1:      "/cache_tier1/kopia-cache",
			tier1Stale: "/data/cache_tier1/kopia-cache",
			tier2:      "/cache_tier2/kopia-cache",
			tier2Stale: "/data/cache_tier2/kopia-cache",
		},
		"fallback to the data volume": {
			tier1: "/data/cache_tier1/kopia-cache",
			tier2: "/data/cache_tier2/kopia-cache",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			builder := &envBuilder{
				tier1: &kliov1alpha1.Tier1Configuration{
					EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
					IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
				},
				tier2: &kliov1alpha1.Tier2Configuration{
					S3:                &kliov1alpha1.S3Configuration{BucketName: "test-bucket"},
					EncryptionKeyFile: newTestFileSource("enc-secret", "encryption-key.age"),
					IdentityFile:      newTestFileSource("id-secret", "identity.txt"),
				},
			}
			if tc.dedicated {
				builder.tier1.Cache = &kliov1alpha1.Cache{}
				builder.tier2.Cache = &kliov1alpha1.Cache{}
			}

			envVars := append(builder.getCoreEnvVars(), builder.getTier2EnvVars()...)

			assert.Equal(t, tc.tier1, findEnvVar(envVars, "TIER1_BASE_CACHE").Value)
			assert.Equal(t, tc.tier1Stale, findEnvVar(envVars, "TIER1_BASE_STALE_CACHE").Value)
			assert.Equal(t, tc.tier2, findEnvVar(envVars, "TIER2_CACHE").Value)
			assert.Equal(t, tc.tier2Stale, findEnvVar(envVars, "TIER2_STALE_CACHE").Value)
		})
	}
}

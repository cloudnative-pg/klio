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

func newTestFileRef(secretName, filePath string) kliov1alpha1.VolumeFileReference {
	return kliov1alpha1.VolumeFileReference{
		Volume: kliov1alpha1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretName},
		},
		Path: filePath,
	}
}

func newTestTLSConfiguration() kliov1alpha1.TLSConfiguration {
	return kliov1alpha1.TLSConfiguration{
		ServerTLSIdentity: kliov1alpha1.TLSIdentity{
			Volume: kliov1alpha1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{Name: "tls-secret"},
						},
					}},
				},
			},
			CertPath: "tls.crt",
			KeyPath:  "tls.key",
		},
		ClientCA: newTestFileRef("ca-secret", "tls.crt"),
	}
}

func TestGetCoreEnvVarsIncludesQueueWhenTier1Configured(t *testing.T) {
	builder := &envBuilder{
		tls: newTestTLSConfiguration(),
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
		},
	}

	envVars := builder.getCoreEnvVars()
	queueDir := findEnvVar(envVars, "QUEUE_DIRECTORY")
	require.NotNil(t, queueDir)
	assert.Equal(t, "/klio/queue", queueDir.Value)
}

func TestGetCoreEnvVarsExcludesQueueWhenNoTier1(t *testing.T) {
	builder := &envBuilder{
		tls:   newTestTLSConfiguration(),
		tier1: nil,
	}

	envVars := builder.getCoreEnvVars()
	queueDir := findEnvVar(envVars, "QUEUE_DIRECTORY")
	assert.Nil(t, queueDir)
}

func TestGetCoreEnvVarsIncludesTier1EnvVars(t *testing.T) {
	builder := &envBuilder{
		tls: newTestTLSConfiguration(),
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
		},
	}

	envVars := builder.getCoreEnvVars()

	for name, want := range map[string]string{
		"TIER1_BASE_CACHE":      "/klio/cache_tier1/kopia-cache",
		"TIER1_BASE_REPOSITORY": "/klio/data/base",
		"TIER1_WAL_PATH":        "/klio/data/wal",
	} {
		env := findEnvVar(envVars, name)
		require.NotNil(t, env, name)
		assert.Equal(t, want, env.Value, name)
	}

	assert.NotNil(t, findEnvVar(envVars, "TIER1_BASE_LISTEN_ADDRESS"))
	assert.NotNil(t, findEnvVar(envVars, "TIER1_WAL_LISTEN_ADDRESS"))

	encKeyFile := findEnvVar(envVars, "TIER1_ENCRYPTION_KEY_FILE")
	require.NotNil(t, encKeyFile)
	assert.Equal(t, "/files/tier1-enc-key-file/encryption-key.age", encKeyFile.Value)

	identityFile := findEnvVar(envVars, "TIER1_IDENTITY_FILE")
	require.NotNil(t, identityFile)
	assert.Equal(t, "/files/tier1-identity/identity.txt", identityFile.Value)
}

func TestGetCoreEnvVarsIncludesTier1Compression(t *testing.T) {
	t.Run("compression set with sizes", func(t *testing.T) {
		builder := &envBuilder{
			tls: newTestTLSConfiguration(),
			tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
				Compression: &kliov1alpha1.CompressionPolicy{
					Algorithm: "zstd",
					MinSize:   4096,
					MaxSize:   1048576,
				},
			},
		}

		envVars := builder.getCoreEnvVars()

		algorithm := findEnvVar(envVars, "TIER1_COMPRESSION_ALGORITHM")
		require.NotNil(t, algorithm)
		assert.Equal(t, "zstd", algorithm.Value)

		minSize := findEnvVar(envVars, "TIER1_COMPRESSION_MIN_SIZE")
		require.NotNil(t, minSize)
		assert.Equal(t, "4096", minSize.Value)

		maxSize := findEnvVar(envVars, "TIER1_COMPRESSION_MAX_SIZE")
		require.NotNil(t, maxSize)
		assert.Equal(t, "1048576", maxSize.Value)
	})

	t.Run("algorithm only omits size vars", func(t *testing.T) {
		builder := &envBuilder{
			tls: newTestTLSConfiguration(),
			tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
				Compression:       &kliov1alpha1.CompressionPolicy{Algorithm: "zstd"},
			},
		}

		envVars := builder.getCoreEnvVars()
		require.NotNil(t, findEnvVar(envVars, "TIER1_COMPRESSION_ALGORITHM"))
		assert.Nil(t, findEnvVar(envVars, "TIER1_COMPRESSION_MIN_SIZE"))
		assert.Nil(t, findEnvVar(envVars, "TIER1_COMPRESSION_MAX_SIZE"))
	})

	t.Run("compression unset", func(t *testing.T) {
		builder := &envBuilder{
			tls: newTestTLSConfiguration(),
			tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
			},
		}

		assert.Nil(t, findEnvVar(builder.getCoreEnvVars(), "TIER1_COMPRESSION_ALGORITHM"))
	})
}

func TestGetTier2EnvVarsIncludesCompression(t *testing.T) {
	t.Run("compression set with sizes", func(t *testing.T) {
		builder := &envBuilder{
			tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName: "test-bucket",
				},
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
				Compression: &kliov1alpha1.CompressionPolicy{
					Algorithm: "s2-default",
					MinSize:   8192,
				},
			},
		}

		envVars := builder.getTier2EnvVars()

		algorithm := findEnvVar(envVars, "TIER2_COMPRESSION_ALGORITHM")
		require.NotNil(t, algorithm)
		assert.Equal(t, "s2-default", algorithm.Value)

		minSize := findEnvVar(envVars, "TIER2_COMPRESSION_MIN_SIZE")
		require.NotNil(t, minSize)
		assert.Equal(t, "8192", minSize.Value)

		assert.Nil(t, findEnvVar(envVars, "TIER2_COMPRESSION_MAX_SIZE"))
	})

	t.Run("compression unset", func(t *testing.T) {
		builder := &envBuilder{
			tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName: "test-bucket",
				},
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
			},
		}

		assert.Nil(t, findEnvVar(builder.getTier2EnvVars(), "TIER2_COMPRESSION_ALGORITHM"))
	})
}

func TestGetTier2EnvVarsS3CredentialsFile(t *testing.T) {
	newBuilder := func(s3 *kliov1alpha1.S3Configuration) *envBuilder {
		return &envBuilder{
			tier2: &kliov1alpha1.Tier2Configuration{
				S3:                s3,
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
			},
		}
	}

	t.Run("file and profile", func(t *testing.T) {
		ref := newTestFileRef("aws-creds", "credentials")
		envVars := newBuilder(&kliov1alpha1.S3Configuration{
			BucketName:      "test-bucket",
			CredentialsFile: &ref,
			Profile:         "klio",
		}).getTier2EnvVars()

		file := findEnvVar(envVars, "AWS_SHARED_CREDENTIALS_FILE")
		require.NotNil(t, file)
		assert.Equal(t, "/files/tier2-s3-credentials/credentials", file.Value)
		profile := findEnvVar(envVars, "AWS_PROFILE")
		require.NotNil(t, profile)
		assert.Equal(t, "klio", profile.Value)
	})

	t.Run("unset", func(t *testing.T) {
		envVars := newBuilder(&kliov1alpha1.S3Configuration{BucketName: "test-bucket"}).getTier2EnvVars()

		assert.Nil(t, findEnvVar(envVars, "AWS_SHARED_CREDENTIALS_FILE"))
		assert.Nil(t, findEnvVar(envVars, "AWS_PROFILE"))
	})
}

func TestGetCoreEnvVarsOnlyTLSWhenNoTier1(t *testing.T) {
	builder := &envBuilder{
		tls:   newTestTLSConfiguration(),
		tier1: nil,
	}

	envVars := builder.getCoreEnvVars()
	assert.Len(t, envVars, 3)
	cert := findEnvVar(envVars, "TLS_CERT")
	require.NotNil(t, cert)
	assert.Equal(t, "/files/server-identity/tls.crt", cert.Value)
	key := findEnvVar(envVars, "TLS_KEY")
	require.NotNil(t, key)
	assert.Equal(t, "/files/server-identity/tls.key", key.Value)
	clientCA := findEnvVar(envVars, "TLS_CLIENT_CA_CERT")
	require.NotNil(t, clientCA)
	assert.Equal(t, "/files/client-ca/tls.crt", clientCA.Value)
}

func TestGetTier2EnvVarsExcludesQueueDirectory(t *testing.T) {
	builder := &envBuilder{
		tls: newTestTLSConfiguration(),
		tier1: &kliov1alpha1.Tier1Configuration{
			EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
		},
		tier2: &kliov1alpha1.Tier2Configuration{
			S3: &kliov1alpha1.S3Configuration{
				BucketName: "test-bucket",
			},
			EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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
			TLSConfiguration: newTestTLSConfiguration(),
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
			},
			Tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName: "test-bucket",
				},
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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
			EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
			IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
		},
	}

	envVars := builder.getTier2EnvVars()

	cache := findEnvVar(envVars, "TIER2_CACHE")
	require.NotNil(t, cache)
	assert.Equal(t, "/klio/cache_tier2/kopia-cache", cache.Value)

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
			TLSConfiguration: newTestTLSConfiguration(),
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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
			TLSConfiguration: newTestTLSConfiguration(),
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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

	klioMount := findMount("klio")
	require.NotNil(t, klioMount)
	assert.Equal(t, "/klio", klioMount.MountPath)
}

func TestBuildFileVolMountCSI(t *testing.T) {
	vol, mount := buildFileVolMount("test-id",
		kliov1alpha1.VolumeFileReference{
			Volume: kliov1alpha1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{Driver: "csi.cert-manager.io"},
			},
			Path: "identity.txt",
		},
	)

	require.NotNil(t, vol.CSI)
	assert.Nil(t, vol.Projected)
	assert.True(t, mount.ReadOnly)
}

func TestBuildRestrictedVolMountSecretDefaults0400(t *testing.T) {
	vol, mount := buildRestrictedVolMount("test-id", newTestFileRef("id-secret", "identity.txt"))

	require.NotNil(t, vol.Secret)
	require.NotNil(t, vol.Secret.DefaultMode)
	assert.Equal(t, int32(0o400), *vol.Secret.DefaultMode)
	assert.True(t, mount.ReadOnly)
}

func TestBuildRestrictedVolMountPreservesUserMode(t *testing.T) {
	mode := int32(0o440)
	vol, _ := buildRestrictedVolMount("test-id",
		kliov1alpha1.VolumeFileReference{
			Volume: kliov1alpha1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "id-secret", DefaultMode: &mode},
			},
			Path: "identity.txt",
		},
	)

	require.NotNil(t, vol.Secret.DefaultMode)
	assert.Equal(t, int32(0o440), *vol.Secret.DefaultMode)
}

func TestBuildRestrictedVolMountConfigMapAndProjected(t *testing.T) {
	vol, _ := buildRestrictedVolMount("test-id",
		kliov1alpha1.VolumeFileReference{
			Volume: kliov1alpha1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cm"},
				},
			},
			Path: "identity.txt",
		},
	)
	require.NotNil(t, vol.ConfigMap.DefaultMode)
	assert.Equal(t, int32(0o400), *vol.ConfigMap.DefaultMode)

	vol, _ = buildRestrictedVolMount("test-id",
		kliov1alpha1.VolumeFileReference{
			Volume: kliov1alpha1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{},
			},
			Path: "identity.txt",
		},
	)
	require.NotNil(t, vol.Projected.DefaultMode)
	assert.Equal(t, int32(0o400), *vol.Projected.DefaultMode)
}

func TestBuildRestrictedVolMountCSIUntouched(t *testing.T) {
	vol, _ := buildRestrictedVolMount("test-id",
		kliov1alpha1.VolumeFileReference{
			Volume: kliov1alpha1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{Driver: "secrets-store.csi.k8s.io"},
			},
			Path: "identity.txt",
		},
	)

	require.NotNil(t, vol.CSI)
	assert.Nil(t, vol.Secret)
}

func TestTier2S3CredentialsVolumeDefaultMode(t *testing.T) {
	creds := newTestFileRef("aws-creds", "credentials")
	caBundle := newTestFileRef("ca-secret", "ca.crt")
	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			TLSConfiguration: newTestTLSConfiguration(),
			Tier2: &kliov1alpha1.Tier2Configuration{
				S3: &kliov1alpha1.S3Configuration{
					BucketName:      "b",
					CredentialsFile: &creds,
					CustomCABundle:  &caBundle,
				},
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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

	credsVol := findVolume(tier2S3CredentialsVolName)
	require.NotNil(t, credsVol)
	require.NotNil(t, credsVol.Secret.DefaultMode)
	assert.Equal(t, int32(0o400), *credsVol.Secret.DefaultMode)

	bundleVol := findVolume(tier2S3CABundleVolName)
	require.NotNil(t, bundleVol)
	assert.Nil(t, bundleVol.Secret.DefaultMode)
}

func TestServerIdentityVolumeDefaultMode(t *testing.T) {
	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			TLSConfiguration: newTestTLSConfiguration(),
			Tier1: &kliov1alpha1.Tier1Configuration{
				EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
				IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
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

	identityVol := findVolume(serverIdentityVolName)
	require.NotNil(t, identityVol)
	require.NotNil(t, identityVol.Projected.DefaultMode)
	assert.Equal(t, int32(0o400), *identityVol.Projected.DefaultMode)

	tier1IdentityVol := findVolume(tier1IdentityVolName)
	require.NotNil(t, tier1IdentityVol)
	require.NotNil(t, tier1IdentityVol.Secret.DefaultMode)
	assert.Equal(t, int32(0o400), *tier1IdentityVol.Secret.DefaultMode)

	encVol := findVolume(tier1EncKeyFileVolName)
	require.NotNil(t, encVol)
	assert.Nil(t, encVol.Secret.DefaultMode)

	clientCAVol := findVolume(clientCAVolName)
	require.NotNil(t, clientCAVol)
	assert.Nil(t, clientCAVol.Secret.DefaultMode)
}

func TestTier2S3CustomCABundle(t *testing.T) {
	ref := kliov1alpha1.VolumeFileReference{
		Volume: kliov1alpha1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{Driver: "secrets-store.csi.k8s.io"},
		},
		Path: "ca.crt",
	}
	tier2 := &kliov1alpha1.Tier2Configuration{
		S3:                &kliov1alpha1.S3Configuration{BucketName: "b", CustomCABundle: &ref},
		EncryptionKeyFile: newTestFileRef("enc-secret", "encryption-key.age"),
		IdentityFile:      newTestFileRef("id-secret", "identity.txt"),
	}

	t.Run("env var points to the mounted file", func(t *testing.T) {
		env := findEnvVar((&envBuilder{tier2: tier2}).getTier2EnvVars(), "TIER2_S3_CUSTOM_CA_BUNDLE_FILE")
		require.NotNil(t, env)
		assert.Equal(t, "/files/tier2-s3-ca-bundle/ca.crt", env.Value)
	})

	r := &ServerReconciler{}
	server := &kliov1alpha1.Server{
		Spec: kliov1alpha1.ServerSpec{
			TLSConfiguration: newTestTLSConfiguration(),
			Tier2:            tier2,
		},
	}

	t.Run("volume and read-only mount", func(t *testing.T) {
		var vol *corev1.Volume
		volumes := r.buildVolumes(server)
		for i := range volumes {
			assert.NotEqual(t, "tier2", volumes[i].Name, "the old projected tier2 volume must be gone")
			if volumes[i].Name == tier2S3CABundleVolName {
				vol = &volumes[i]
			}
		}
		require.NotNil(t, vol)
		require.NotNil(t, vol.CSI)
		assert.Equal(t, "secrets-store.csi.k8s.io", vol.CSI.Driver)

		var mount *corev1.VolumeMount
		for _, m := range r.buildVolumeMounts(server) {
			if m.Name == tier2S3CABundleVolName {
				mount = &m
			}
		}
		require.NotNil(t, mount)
		assert.Equal(t, "/files/tier2-s3-ca-bundle", mount.MountPath)
		assert.True(t, mount.ReadOnly)
	})

	t.Run("nothing is mounted when unset", func(t *testing.T) {
		server.Spec.Tier2 = &kliov1alpha1.Tier2Configuration{
			S3:                &kliov1alpha1.S3Configuration{BucketName: "b"},
			EncryptionKeyFile: tier2.EncryptionKeyFile,
			IdentityFile:      tier2.IdentityFile,
		}
		for _, v := range r.buildVolumes(server) {
			assert.NotEqual(t, tier2S3CABundleVolName, v.Name)
		}
	})
}

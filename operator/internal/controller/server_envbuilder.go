package controller

import (
	"path"

	machineryapi "github.com/cloudnative-pg/machinery/pkg/api"
	corev1 "k8s.io/api/core/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

const kopiaCacheSubdirectory = "kopia-cache"

type envBuilder struct {
	builtEnvs []corev1.EnvVar

	tier1 *kliov1alpha1.Tier1Configuration
	tier2 *kliov1alpha1.Tier2Configuration
}

func newServerEnvBuilder(server *kliov1alpha1.Server) *envBuilder {
	return &envBuilder{
		tier1: server.Spec.Tier1,
		tier2: server.Spec.Tier2,
	}
}

func (e *envBuilder) build() []corev1.EnvVar {
	return e.builtEnvs
}

func (e *envBuilder) addCommonEnvs() *envBuilder {
	result := e.getCoreEnvVars()
	result = append(result, e.getKubernetesDownwardAPIEnvVars()...)
	result = append(result, e.getTier2EnvVars()...)

	e.builtEnvs = append(e.builtEnvs, result...)

	return e
}

func (e *envBuilder) addServerEnvs() *envBuilder {
	e.builtEnvs = append(e.builtEnvs, corev1.EnvVar{Name: "CONTAINER_NAME", Value: "base"})
	return e
}

// getKubernetesDownwardAPIEnvVars provides Kubernetes metadata through the downward API.
func (e *envBuilder) getKubernetesDownwardAPIEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "NAMESPACE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}
}

// fileSourcePath computes the container file path for a FileSource volume.
func fileSourcePath(volName string, src kliov1alpha1.FileSource) string {
	return path.Join(fileSourceBasePath, volName, src.FileReference.Path)
}

func (e *envBuilder) getCoreEnvVars() []corev1.EnvVar {
	basePath := path.Join(kopiaDataMountPath, "base")
	walPath := path.Join(kopiaDataMountPath, "wal")

	result := []corev1.EnvVar{
		{
			Name:  "TLS_CERT",
			Value: "/certs/tls.crt",
		},
		{
			Name:  "TLS_KEY",
			Value: "/certs/tls.key",
		},
		{
			Name:  "TLS_CLIENT_CA_CERT",
			Value: "/client-ca/tls.crt",
		},
	}

	if e.tier1 != nil {
		tier1Envs := make([]corev1.EnvVar, 0, 8) //nolint:mnd
		tier1Envs = append(tier1Envs,
			corev1.EnvVar{
				Name:  "TIER1_BASE_CACHE",
				Value: path.Join(kopiaCacheTier1MountPath, kopiaCacheSubdirectory),
			},
			corev1.EnvVar{
				Name:  "TIER1_BASE_REPOSITORY",
				Value: basePath,
			},
			corev1.EnvVar{
				Name:  "TIER1_BASE_LISTEN_ADDRESS",
				Value: "0.0.0.0:51515",
			},
			corev1.EnvVar{
				Name:  "TIER1_WAL_LISTEN_ADDRESS",
				Value: "0.0.0.0:52000",
			},
			corev1.EnvVar{
				Name:  "TIER1_WAL_PATH",
				Value: walPath,
			},
		)

		tier1Envs = append(tier1Envs,
			corev1.EnvVar{
				Name:  "TIER1_ENCRYPTION_KEY_FILE",
				Value: fileSourcePath(tier1EncKeyFileVolName, e.tier1.EncryptionKeyFile),
			},
			corev1.EnvVar{
				Name:  "TIER1_IDENTITY_FILE",
				Value: fileSourcePath(tier1IdentityVolName, e.tier1.IdentityFile),
			},
		)

		// The queue is always required when tier1 is enabled to support
		// retention policy enforcement.
		tier1Envs = append(tier1Envs, corev1.EnvVar{
			Name:  "QUEUE_DIRECTORY",
			Value: "/queue",
		})

		result = append(result, tier1Envs...)
	}

	return result
}

func (e *envBuilder) getTier2EnvVars() []corev1.EnvVar {
	if e.tier2 == nil {
		return nil
	}
	if e.tier2.S3 == nil {
		return nil
	}

	result := []corev1.EnvVar{
		{
			Name:  "TIER2_S3_ENABLED",
			Value: "true",
		},
		{
			Name:  "TIER2_S3_BUCKET_NAME",
			Value: e.tier2.S3.BucketName,
		},
		{
			Name:  "TIER2_CACHE",
			Value: path.Join(kopiaCacheTier2MountPath, kopiaCacheSubdirectory),
		},
		{
			Name:  "TIER2_BASE_LISTEN_ADDRESS",
			Value: "0.0.0.0:51516",
		},
		{
			Name:  "TIER2_WAL_LISTEN_ADDRESS",
			Value: "0.0.0.0:52001",
		},
	}

	result = append(result,
		corev1.EnvVar{
			Name:  "TIER2_ENCRYPTION_KEY_FILE",
			Value: fileSourcePath(tier2EncKeyFileVolName, e.tier2.EncryptionKeyFile),
		},
		corev1.EnvVar{
			Name:  "TIER2_IDENTITY_FILE",
			Value: fileSourcePath(tier2IdentityVolName, e.tier2.IdentityFile),
		},
	)

	// Inject explicit credentials if provided. When credentials are not provided,
	// the AWS SDK will automatically use IAM role credentials (EKS IRSA, or Pod Identity).
	if e.tier2.S3.AccessKeyID != nil {
		result = append(result, corev1.EnvVar{
			Name:      "TIER2_S3_ACCESS_KEY_ID",
			ValueFrom: secretKeySelectorToEnvVarSource(e.tier2.S3.AccessKeyID),
		})
	}
	if e.tier2.S3.SecretAccessKey != nil {
		result = append(result, corev1.EnvVar{
			Name:      "TIER2_S3_SECRET_ACCESS_KEY",
			ValueFrom: secretKeySelectorToEnvVarSource(e.tier2.S3.SecretAccessKey),
		})
	}
	if e.tier2.S3.SessionToken != nil {
		result = append(result, corev1.EnvVar{
			Name:      "TIER2_S3_SESSION_TOKEN",
			ValueFrom: secretKeySelectorToEnvVarSource(e.tier2.S3.SessionToken),
		})
	}
	if e.tier2.S3.CustomCABundle != nil {
		result = append(result, corev1.EnvVar{
			Name:  "TIER2_S3_CUSTOM_CA_BUNDLE_FILE",
			Value: "/tier2/custom_ca_bundle.pem",
		})
	}

	// Add optional S3 configuration environment variables.
	result = appendEnvIfNotEmpty(result, "TIER2_S3_ENDPOINT", e.tier2.S3.Endpoint)
	result = appendEnvIfNotEmpty(result, "TIER2_S3_PREFIX", e.tier2.S3.Prefix)
	result = appendEnvIfNotEmpty(result, "TIER2_S3_REGION", e.tier2.S3.Region)

	return result
}

func appendEnvIfNotEmpty(envs []corev1.EnvVar, name, value string) []corev1.EnvVar {
	if value != "" {
		return append(envs, corev1.EnvVar{Name: name, Value: value})
	}

	return envs
}

func secretKeySelectorToEnvVarSource(src *machineryapi.SecretKeySelector) *corev1.EnvVarSource {
	if src == nil {
		return nil
	}

	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: src.Name,
			},
			Key: src.Key,
		},
	}
}

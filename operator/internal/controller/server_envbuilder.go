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

func newRecoverySourceEnvBuilder(recoverySource *kliov1alpha1.RecoverySource) *envBuilder {
	return &envBuilder{
		tier1: nil,
		tier2: &recoverySource.Spec.Tier2,
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
		tier1Envs := []corev1.EnvVar{
			{
				Name:  "TIER1_BASE_CACHE",
				Value: path.Join(kopiaCacheTier1MountPath, kopiaCacheSubdirectory),
			},
			{
				Name:  "TIER1_BASE_REPOSITORY",
				Value: basePath,
			},
			{
				Name:  "TIER1_BASE_LISTEN_ADDRESS",
				Value: "0.0.0.0:51515",
			},
			{
				Name:  "TIER1_WAL_LISTEN_ADDRESS",
				Value: "0.0.0.0:52000",
			},
			{
				Name:  "TIER1_WAL_PATH",
				Value: walPath,
			},
			{
				Name:      "TIER1_ENCRYPTION_KEY",
				ValueFrom: secretKeySelectorToEnvVarSource(e.tier1.EncryptionKey),
			},
		}
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
			Name:  "TIER2_S3_ENDPOINT",
			Value: e.tier2.S3.Endpoint,
		},
		{
			Name:  "TIER2_S3_PREFIX",
			Value: e.tier2.S3.Prefix,
		},
		{
			Name:      "TIER2_ENCRYPTION_KEY",
			ValueFrom: secretKeySelectorToEnvVarSource(e.tier2.EncryptionKey),
		},
		{
			Name:  "TIER2_S3_REGION",
			Value: e.tier2.S3.Region,
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

	if e.tier1 != nil {
		result = append(result, corev1.EnvVar{
			Name:  "QUEUE_DIRECTORY",
			Value: "/queue",
		})
	}

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

	return result
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

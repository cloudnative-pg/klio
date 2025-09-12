package controller

import (
	"path"

	corev1 "k8s.io/api/core/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

type envBuilder struct {
	builtEnvs []corev1.EnvVar
	server    *kliov1alpha1.Server
}

func newEnvBuilder(server *kliov1alpha1.Server) *envBuilder {
	return &envBuilder{server: server}
}

func (e *envBuilder) build() []corev1.EnvVar {
	return e.builtEnvs
}

func (e *envBuilder) addCommonEnvs() *envBuilder {
	result := e.getCoreEnvVars()
	result = append(result, e.getKubernetesDownwardAPIEnvVars()...)
	result = append(result, e.getAdminUserEnvVars()...)
	result = append(result, e.server.Spec.BaseConfiguration.Envs.Common...)

	e.builtEnvs = append(e.builtEnvs, result...)

	return e
}

func (e *envBuilder) addWalEnv() *envBuilder {
	e.builtEnvs = append(e.builtEnvs, e.server.Spec.BaseConfiguration.Envs.WAL...)

	return e
}

func (e *envBuilder) addBaseEnv() *envBuilder {
	e.builtEnvs = append(e.builtEnvs, e.server.Spec.BaseConfiguration.Envs.Base...)
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
			Name: "POD_UID",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.uid",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
	}
}

func (e *envBuilder) getCoreEnvVars() []corev1.EnvVar {
	basePath := path.Join(kopiaDataMountPath, "base")
	walPath := path.Join(kopiaDataMountPath, "wal")

	return []corev1.EnvVar{
		{
			Name: "BASE_ENCRYPTION_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: e.server.Spec.Password.Name,
					},
					Key: e.server.Spec.Password.Key,
				},
			},
		},
		{Name: "BASE_CACHE", Value: kopiaCacheMountPath},
		{Name: "BASE_REPOSITORY", Value: basePath},
		{Name: "BASE_TLS_CERT", Value: "/certs/tls.crt"},
		{Name: "BASE_TLS_KEY", Value: "/certs/tls.key"},
		{Name: "BASE_LISTEN_ADDRESS", Value: "0.0.0.0:51515"},
		{Name: "BASE_HTPASSWD_FILE", Value: path.Join(kopiaConfigMountPath, htpasswdFileName)},
		{Name: "WAL_LISTEN_ADDRESS", Value: "0.0.0.0:52000"},
		{Name: "WAL_TLS_CERT", Value: "/certs/tls.crt"},
		{Name: "WAL_TLS_KEY", Value: "/certs/tls.key"},
		{Name: "WAL_PATH", Value: walPath},
		{
			Name: "WAL_ENCRYPTION_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: e.server.Spec.Password.Name,
					},
					Key: e.server.Spec.Password.Key,
				},
			},
		},
		{Name: "WAL_HTPASSWD_FILE", Value: path.Join(kopiaConfigMountPath, htpasswdFileName)},
	}
}

func (e *envBuilder) getAdminUserEnvVars() []corev1.EnvVar {
	if e.server.Spec.BaseConfiguration.AdminUser.Name == "" {
		return nil
	}

	return []corev1.EnvVar{
		{
			Name: "BASE_ADMIN_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: e.server.Spec.BaseConfiguration.AdminUser.Name,
					},
					Key: corev1.BasicAuthUsernameKey,
				},
			},
		},
		{
			Name: "BASE_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: e.server.Spec.BaseConfiguration.AdminUser.Name,
					},
					Key: corev1.BasicAuthPasswordKey,
				},
			},
		},
	}
}

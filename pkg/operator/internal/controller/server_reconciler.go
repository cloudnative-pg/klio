package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kliov1alpha1 "github.com/cloudnative-pg/klio/pkg/operator/api/v1alpha1"
)

const typeLabel = "klio.cnpg.io/type"
const kopiaTypeLabelValue = "kopia"
const klioServerLabel = "klio.cnpg.io/klio-server"

func (r *ServerReconciler) reconcile(ctx context.Context, server *kliov1alpha1.Server) error {
	if err := r.reconcileStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}

	return nil
}

func (r *ServerReconciler) reconcileStatefulSet(ctx context.Context, server *kliov1alpha1.Server) error {
	kopiaEnv := []corev1.EnvVar{
		{
			Name: "KOPIA_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: server.Spec.KopiaConfiguration.Password.Name,
					},
					Key: server.Spec.KopiaConfiguration.Password.Key,
				},
			},
		},
		{Name: "KOPIA_CONFIG_PATH", Value: "/data/kopia.config"},
		{Name: "KOPIA_LOG_DIR", Value: server.Spec.KopiaConfiguration.LogDirectory},
		{Name: "KOPIA_CACHE_DIRECTORY", Value: server.Spec.KopiaConfiguration.CacheDirectory},
		{Name: "USER", Value: server.Spec.KopiaConfiguration.User},
	}

	klioEnv := r.getKlioEnvs(server)
	kopiaName := server.Name + "-kopia"

	expected := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kopiaName,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerLabel: server.Name,
				typeLabel:       kopiaTypeLabelValue,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: kopiaName,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
						Labels: map[string]string{
							klioServerLabel: server.Name,
							typeLabel:       kopiaTypeLabelValue,
						},
					},
					Spec: server.Spec.PersistentVolumeClaimTemplate,
				},
			},
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					klioServerLabel: server.Name,
					typeLabel:       kopiaTypeLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						klioServerLabel: server.Name,
						typeLabel:       kopiaTypeLabelValue,
					},
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: server.Spec.ImagePullSecrets,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:      ptr.To(int64(1000)),
						RunAsGroup:   ptr.To(int64(1000)),
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(1000)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "klio-initialize",
							Command:         []string{"klio", "initialize", "--config=/config/klio.yaml"},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
								{Name: "config", MountPath: "/config"},
							},
						},
						{
							Name: "kopia-initialize",
							Command: []string{"sh", "-c",
								"(test ! -d /data/pgdata && kopia repository create filesystem --path=/data/pgdata) ||:"},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
							},
							Env: kopiaEnv,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "klio-server",
							Command:         []string{"klio", "serve"},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							Resources:       server.Spec.Resources,
							Ports: []corev1.ContainerPort{
								{Name: "klio", ContainerPort: 52000, Protocol: corev1.ProtocolTCP},
								{Name: "kopia", ContainerPort: 51515, Protocol: corev1.ProtocolTCP},
							},
							Env: klioEnv,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
								{Name: "tls", MountPath: "/certs"},
							},
						},
						// TODO: remove hardcoded
						{
							Name: "kopia-server",
							Command: []string{
								"kopia", "server", "start",
								"--address=0.0.0.0:51515",
								"--server-username=klio@cluster-example",
								"--server-password=CHANGE_ME_KOPIA_PASSWORD
								"--server-control-username=klio",
								"--server-control-password=CHANGE_ME_KOPIA_PASSWORD
								"--tls-cert-file=/certs/tls.crt",
								"--tls-key-file=/certs/tls.key",
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							Resources:       server.Spec.KopiaConfiguration.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
								{Name: "tls", MountPath: "/certs"},
							},
							Env: kopiaEnv,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: server.Name,
								},
							},
						},
						{
							Name: "tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: server.Spec.TLSSecretName,
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: corev1.StorageMediumDefault,
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{},
	}

	// TODO improve
	type hashBuilder struct {
		sts     *appsv1.StatefulSet
		version int
	}

	hash, err := ComputeHash(hashBuilder{sts: expected, version: 1})
	if err != nil {
		return fmt.Errorf("failed to compute hash for Kopia configuration: %w", err)
	}

	expected.Annotations = map[string]string{
		"klio.cnpg.io/klio-server-hash": hash,
	}

	if err := ctrl.SetControllerReference(server, expected, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	var current appsv1.StatefulSet
	err = r.Get(ctx, client.ObjectKeyFromObject(expected), &current)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		return r.Create(ctx, expected)
	}

	if !metav1.IsControlledBy(&current, server) {
		return fmt.Errorf("StatefulSet is not owned by Server %s/%s", server.Namespace, server.Name)
	}

	if current.Annotations["klio.cnpg.io/klio-server-hash"] != hash {
		return r.Update(ctx, expected)
	}

	return nil
}

func (r *ServerReconciler) getKlioEnvs(server *kliov1alpha1.Server) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "KLIO_SERVER_LISTEN_ADDRESS", Value: "0.0.0.0:52000"},
		{Name: "KLIO_SERVER_SERVER_CERT_PATH", Value: "/certs/tls.crt"},
		{Name: "KLIO_SERVER_SERVER_KEY_PATH", Value: "/certs/tls.key"},
		{Name: "KLIO_SERVER_WAL_PATH", Value: "/data/wals"},
		{
			Name: "KLIO_SERVER_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: server.Spec.Password.Name,
					},
					Key: server.Spec.Password.Key,
				},
			},
		},
	}
}

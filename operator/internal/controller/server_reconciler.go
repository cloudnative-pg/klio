package controller

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

const (
	pvcTypeLabel       = "klio.cnpg.io/pvcType"
	typeLabel          = "klio.cnpg.io/type"
	baseTypeLabelValue = "base"
	klioServerLabel    = "klio.cnpg.io/klio-server"
)

const (
	kopiaDataMountPath   = "/data"
	kopiaCacheMountPath  = "/cache"
	htpasswdFileName     = "htpasswd"
	kopiaConfigMountPath = "/config"
	otelConfigMountPath  = "/otel"
)

func (r *ServerReconciler) reconcile(ctx context.Context, server *kliov1alpha1.Server) error {
	if err := r.reconcileStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}

	if err := r.reconcileService(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	return nil
}

//nolint:cyclop
func (r *ServerReconciler) reconcileStatefulSet(ctx context.Context, server *kliov1alpha1.Server) error {
	klioName := server.Name + "-klio"

	pprof, _ := strconv.ParseBool(server.GetAnnotations()["klio.cnpg.io/pprof"])

	// build volumes including any OTEL certificate volumes
	volumes := r.buildVolumes(server)
	volumeMounts := r.buildVolumeMounts(server)

	expected := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioName,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerLabel: server.Name,
				typeLabel:       baseTypeLabelValue,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: klioName,
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
						Labels: map[string]string{
							klioServerLabel: server.Name,
							pvcTypeLabel:    "data",
						},
					},
					Spec: server.Spec.DataConfiguration.PersistentVolumeClaimTemplate,
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cache",
						Labels: map[string]string{
							klioServerLabel: server.Name,
							pvcTypeLabel:    "cache",
						},
					},
					Spec: server.Spec.CacheConfiguration.PersistentVolumeClaimTemplate,
				},
			},
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					klioServerLabel: server.Name,
					typeLabel:       baseTypeLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						klioServerLabel: server.Name,
						typeLabel:       baseTypeLabelValue,
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
							Name: "init",
							Args: []string{
								"server",
								"initialize",
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							VolumeMounts:    volumeMounts,
							Env:             newEnvBuilder(server).addCommonEnvs().addBaseEnv().build(),
						},
					},
					Containers: []corev1.Container{
						{
							Name: "base",
							Args: []string{
								"server",
								"start-base",
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							Resources:       server.Spec.BaseConfiguration.Resources,
							VolumeMounts:    volumeMounts,
							Ports: []corev1.ContainerPort{
								{Name: "base", ContainerPort: 51515, Protocol: corev1.ProtocolTCP},
							},
							Env: newEnvBuilder(server).addCommonEnvs().addBaseEnv().build(),
						},
						{
							Name: "wal",
							Args: []string{
								"server",
								"start-wal",
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							Resources:       server.Spec.Resources,
							Ports: []corev1.ContainerPort{
								{Name: "wal", ContainerPort: 52000, Protocol: corev1.ProtocolTCP},
							},
							Env:          newEnvBuilder(server).addCommonEnvs().addWalEnv().build(),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{},
	}

	if pprof {
		for i := range expected.Spec.Template.Spec.Containers {
			expected.Spec.Template.Spec.Containers[i].Args = append(
				expected.Spec.Template.Spec.Containers[i].Args,
				"--pprof-server=:606"+strconv.Itoa(i))
		}
	}

	//nolint:godox
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
			return fmt.Errorf("failed to get statefulset %v/%v: %w", expected.Namespace, expected.Name, err)
		}

		if err := r.Create(ctx, expected); err != nil {
			return fmt.Errorf("failed to create StatefulSet %s/%s: %w", expected.Namespace, expected.Name, err)
		}

		return nil
	}

	if !metav1.IsControlledBy(&current, server) {
		return &statefulSetNotOwnedByServerError{
			ServerName:      server.Name,
			ServerNamespace: server.Namespace,
		}
	}

	if current.Annotations["klio.cnpg.io/klio-server-hash"] != hash {
		if err := r.Update(ctx, expected); err != nil {
			return fmt.Errorf("failed to update StatefulSet %s/%s: %w", expected.Namespace, expected.Name, err)
		}

		return nil
	}

	return nil
}

func (r *ServerReconciler) reconcileService(ctx context.Context, server *kliov1alpha1.Server) error {
	expected := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerLabel: server.Name,
				typeLabel:       baseTypeLabelValue,
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name: "base",
					Port: 51515,
				},
				{
					Name: "wal",
					Port: 52000,
				},
			},
			Selector: map[string]string{
				klioServerLabel: server.Name,
				typeLabel:       baseTypeLabelValue,
			},
		},
	}

	err := ctrl.SetControllerReference(server, expected, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	var current corev1.Service
	err = r.Get(ctx, client.ObjectKeyFromObject(expected), &current)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get service %s/%s: %w", expected.Namespace, expected.Name, err)
		}

		if err := r.Create(ctx, expected); err != nil {
			return fmt.Errorf("failed to create Service %s/%s: %w", expected.Namespace, expected.Name, err)
		}

		return nil
	}

	if !metav1.IsControlledBy(&current, server) {
		return &serviceNotOwnedByServerError{
			ServerName:      server.Name,
			ServerNamespace: server.Namespace,
		}
	}

	if err := r.Update(ctx, expected); err != nil {
		return fmt.Errorf("failed to update service %s/%s: %w", expected.Namespace, expected.Name, err)
	}

	return nil
}

func (r *ServerReconciler) buildVolumes(server *kliov1alpha1.Server) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: server.Spec.TLSSecretName,
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							Secret: &corev1.SecretProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: server.Spec.Users.Name,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  "htpasswd",
										Path: htpasswdFileName,
									},
								},
								Optional: ptr.To(false),
							},
						},
					},
				},
			},
		},
	}

	if server.Spec.ShouldCreateOtelVolume() {
		volumes = append(volumes, corev1.Volume{
			Name: "otel",
			VolumeSource: corev1.VolumeSource{
				Projected: server.Spec.Observability.OpenTelemetry.ProjectedSource,
			},
		})
	}

	return volumes
}

func (r *ServerReconciler) buildVolumeMounts(server *kliov1alpha1.Server) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: kopiaDataMountPath},
		{Name: "tls", MountPath: "/certs"},
		{Name: "cache", MountPath: kopiaCacheMountPath},
		{Name: "tmp", MountPath: "/tmp"},
		{Name: "config", MountPath: kopiaConfigMountPath},
	}

	if server.Spec.ShouldCreateOtelVolume() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "otel",
			MountPath: otelConfigMountPath,
			ReadOnly:  true,
		})
	}

	return volumeMounts
}

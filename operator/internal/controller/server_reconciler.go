package controller

import (
	"context"
	"fmt"
	"maps"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/podtemplate"
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
)

func (r *ServerReconciler) reconcile(ctx context.Context, server *kliov1alpha1.Server) error {
	if err := r.reconcileService(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	if err := r.reconcileStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}

	return nil
}

//nolint:cyclop
func (r *ServerReconciler) reconcileStatefulSet(ctx context.Context, server *kliov1alpha1.Server) error {
	klioName := server.Name + "-klio"

	pprof, _ := strconv.ParseBool(server.GetAnnotations()["klio.cnpg.io/pprof"])

	volumes := r.buildVolumes(server)
	volumeMounts := r.buildVolumeMounts()

	expected := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioName,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerLabel: server.Name,
				typeLabel:       baseTypeLabelValue,
			},
			Annotations: map[string]string{},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: server.GetServiceName(),
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
								"--skip-if-existing",
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							VolumeMounts:    volumeMounts,
							Env:             newEnvBuilder(server).addCommonEnvs().addInitEnvs().build(),
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
							Env: newEnvBuilder(server).addCommonEnvs().addBaseEnvs().build(),
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
							Env:          newEnvBuilder(server).addCommonEnvs().addWalEnvs().build(),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{},
	}

	if server.Spec.Template != nil {
		merged, err := podtemplate.Merge(&expected.Spec.Template, server.Spec.Template)
		if err != nil {
			return fmt.Errorf("failed to merge pod templates: %w", err)
		}
		expected.Spec.Template = *merged

		// Enforce required labels after merge to keep selector/template consistent
		if expected.Spec.Template.Labels == nil {
			expected.Spec.Template.Labels = map[string]string{}
		}
		expected.Spec.Template.Labels[klioServerLabel] = server.Name
		expected.Spec.Template.Labels[typeLabel] = baseTypeLabelValue
	}

	// Append pprof args after merge so overlay replacements of Args cannot drop them
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

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioName,
			Namespace: server.Namespace,
		},
	}
	op, err := ctrl.CreateOrUpdate(ctx, r.Client, statefulset, func() error {
		if !statefulset.CreationTimestamp.IsZero() && !metav1.IsControlledBy(statefulset, server) {
			return &statefulSetNotOwnedByServerError{
				ServerName:      server.Name,
				ServerNamespace: server.Namespace,
			}
		}

		if statefulset.Annotations["klio.cnpg.io/klio-server-hash"] == hash {
			return nil
		}

		if err := ctrl.SetControllerReference(server, statefulset, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		if statefulset.Labels == nil {
			statefulset.Labels = map[string]string{}
		}
		if statefulset.Annotations == nil {
			statefulset.Annotations = map[string]string{}
		}

		maps.Copy(statefulset.Labels, expected.Labels)
		maps.Copy(statefulset.Annotations, expected.Annotations)
		statefulset.Spec = expected.Spec

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile statefulset (%s) %s/%s: %w", op, statefulset.Namespace, statefulset.Name, err)
	}

	return nil
}

func (r *ServerReconciler) reconcileService(ctx context.Context, server *kliov1alpha1.Server) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.GetServiceName(),
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		if !service.CreationTimestamp.IsZero() && !metav1.IsControlledBy(service, server) {
			return &serviceNotOwnedByServerError{
				ServerName:      server.Name,
				ServerNamespace: server.Namespace,
			}
		}

		if service.Labels == nil {
			service.Labels = map[string]string{}
		}
		if service.Annotations == nil {
			service.Annotations = map[string]string{}
		}

		maps.Copy(service.Labels, map[string]string{
			klioServerLabel: server.Name,
			typeLabel:       baseTypeLabelValue,
		})

		service.Spec.SessionAffinity = corev1.ServiceAffinityNone

		service.Spec.Selector = map[string]string{
			klioServerLabel: server.Name,
			typeLabel:       baseTypeLabelValue,
		}
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name: "base",
				Port: 51515,
			},
			{
				Name: "wal",
				Port: 52000,
			},
		}
		service.Spec.ClusterIP = "None"

		err := ctrl.SetControllerReference(server, service, r.Scheme)
		if err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile service (%s) %s/%s: %w", op, service.Namespace, service.Name, err)
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

	return volumes
}

func (r *ServerReconciler) buildVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: kopiaDataMountPath},
		{Name: "tls", MountPath: "/certs"},
		{Name: "cache", MountPath: kopiaCacheMountPath},
		{Name: "tmp", MountPath: "/tmp"},
		{Name: "config", MountPath: kopiaConfigMountPath},
	}

	return volumeMounts
}

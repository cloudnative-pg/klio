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
	klioRecoverySourceLabel = "klio.cnpg.io/klio-recovery-source"
)

func (r *RecoverySourceReconciler) reconcile(ctx context.Context, recoverySource *kliov1alpha1.RecoverySource) error {
	if err := r.reconcileService(ctx, recoverySource); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	if err := r.reconcileStatefulSet(ctx, recoverySource); err != nil {
		return fmt.Errorf("failed to reconcile StatefulSet: %w", err)
	}

	return nil
}

func (r *RecoverySourceReconciler) reconcileService(
	ctx context.Context,
	recoverySource *kliov1alpha1.RecoverySource,
) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      recoverySource.GetServiceName(),
			Namespace: recoverySource.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		if !service.CreationTimestamp.IsZero() && !metav1.IsControlledBy(service, recoverySource) {
			return newIncorrectOwnershipError(service, recoverySource)
		}

		if service.Labels == nil {
			service.Labels = map[string]string{}
		}

		maps.Copy(service.Labels, map[string]string{
			klioRecoverySourceLabel: recoverySource.Name,
			typeLabel:               baseTypeLabelValue,
		})

		service.Spec.SessionAffinity = corev1.ServiceAffinityNone

		service.Spec.Selector = map[string]string{
			klioRecoverySourceLabel: recoverySource.Name,
			typeLabel:               baseTypeLabelValue,
		}
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name: "tier2-base",
				Port: 51516,
			},
			{
				Name: "tier2-wal",
				Port: 52001,
			},
		}
		service.Spec.ClusterIP = "None"

		err := ctrl.SetControllerReference(recoverySource, service, r.Scheme)
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

//nolint:cyclop
func (r *RecoverySourceReconciler) reconcileStatefulSet(
	ctx context.Context,
	recoverySource *kliov1alpha1.RecoverySource,
) error {
	klioName := recoverySource.Name + "-klio"

	pprof, _ := strconv.ParseBool(recoverySource.GetAnnotations()["klio.cnpg.io/pprof"])

	volumes := r.buildVolumes(recoverySource)
	volumeMounts := r.buildVolumeMounts()

	expected := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioName,
			Namespace: recoverySource.Namespace,
			Labels: map[string]string{
				klioRecoverySourceLabel: recoverySource.Name,
				typeLabel:               baseTypeLabelValue,
			},
			Annotations: map[string]string{},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: recoverySource.GetServiceName(),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cache",
						Labels: map[string]string{
							klioRecoverySourceLabel: recoverySource.Name,
							pvcTypeLabel:            "cache",
						},
					},
					Spec: recoverySource.Spec.Storage.Cache.PersistentVolumeClaimTemplate,
				},
			},
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					klioRecoverySourceLabel: recoverySource.Name,
					typeLabel:               baseTypeLabelValue,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						klioRecoverySourceLabel: recoverySource.Name,
						typeLabel:               baseTypeLabelValue,
					},
				},
				Spec: corev1.PodSpec{
					ImagePullSecrets: recoverySource.Spec.ImagePullSecrets,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:      ptr.To(int64(1000)),
						RunAsGroup:   ptr.To(int64(1000)),
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(int64(1000)),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name: "tier2-base",
							Args: []string{
								"tier2",
								"start-base",
							},
							Image:           recoverySource.Spec.Image,
							ImagePullPolicy: recoverySource.Spec.ImagePullPolicy,
							Env:             newRecoverySourceEnvBuilder(recoverySource).addCommonEnvs().addTier2BaseEnvs().build(),
							Ports: []corev1.ContainerPort{
								{Name: "tier2-base", ContainerPort: 51516, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: volumeMounts,
						},
						{
							Name: "tier2-wal",
							Args: []string{
								"tier2",
								"start-wal",
							},
							Image:           recoverySource.Spec.Image,
							ImagePullPolicy: recoverySource.Spec.ImagePullPolicy,
							Env:             newRecoverySourceEnvBuilder(recoverySource).addCommonEnvs().addTier2WalEnvs().build(),
							VolumeMounts:    volumeMounts,
							Ports: []corev1.ContainerPort{
								{Name: "tier2-wal", ContainerPort: 52001, Protocol: corev1.ProtocolTCP},
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{},
	}

	if recoverySource.Spec.Template != nil {
		merged, err := podtemplate.Merge(&expected.Spec.Template, recoverySource.Spec.Template)
		if err != nil {
			return fmt.Errorf("failed to merge pod templates: %w", err)
		}
		expected.Spec.Template = *merged

		// Enforce required labels after merge to keep selector/template consistent
		if expected.Spec.Template.Labels == nil {
			expected.Spec.Template.Labels = map[string]string{}
		}
		expected.Spec.Template.Labels[klioRecoverySourceLabel] = recoverySource.Name
		expected.Spec.Template.Labels[typeLabel] = baseTypeLabelValue
	}

	// Append pprof args after merge so overlay replacements of Args cannot drop them
	if pprof {
		enablePProf(expected.Spec.Template.Spec.Containers)
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
		"klio.cnpg.io/klio-recovery-source-hash": hash,
	}

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      klioName,
			Namespace: recoverySource.Namespace,
		},
	}
	op, err := ctrl.CreateOrUpdate(ctx, r.Client, statefulset, func() error {
		if !statefulset.CreationTimestamp.IsZero() && !metav1.IsControlledBy(statefulset, recoverySource) {
			return newIncorrectOwnershipError(statefulset, recoverySource)
		}

		if statefulset.Annotations["klio.cnpg.io/klio-recovery-source-hash"] == hash {
			return nil
		}

		if err := ctrl.SetControllerReference(recoverySource, statefulset, r.Scheme); err != nil {
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
		return fmt.Errorf("failed to reconcile statefulset (%s) %s/%s: %w",
			op,
			statefulset.Namespace,
			statefulset.Name,
			err)
	}

	return nil
}

func (r *RecoverySourceReconciler) buildVolumes(recoverySource *kliov1alpha1.RecoverySource) []corev1.Volume {
	volumes := make([]corev1.Volume, 3, 5)
	volumes[0] = corev1.Volume{
		Name: "tls",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: recoverySource.Spec.TLSSecretName,
			},
		},
	}
	volumes[1] = corev1.Volume{
		Name: "client-ca",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: recoverySource.Spec.ClientCASecretName,
			},
		},
	}
	volumes[2] = corev1.Volume{
		Name: "tmp",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}

	var sources []corev1.VolumeProjection

	if recoverySource.Spec.Tier2.S3 != nil && recoverySource.Spec.Tier2.S3.CustomCABundle != nil {
		sources = append(sources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: recoverySource.Spec.Tier2.S3.CustomCABundle.Name,
				},
				Items: []corev1.KeyToPath{
					{
						Path: "custom_ca_bundle.pem",
						Key:  recoverySource.Spec.Tier2.S3.CustomCABundle.Key,
					},
				},
			},
		})
	}

	volumes = append(
		volumes,
		corev1.Volume{
			Name: "tier2",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: sources,
				},
			},
		},
	)

	return volumes
}

func (r *RecoverySourceReconciler) buildVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tls",
			MountPath: "/certs",
		},
		{
			Name:      "client-ca",
			MountPath: "/client-ca",
		},
		{
			Name:      "cache",
			MountPath: kopiaCacheMountPath,
		},
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
		{
			Name:      "tier2",
			MountPath: "/tier2",
		},
	}

	return volumeMounts
}

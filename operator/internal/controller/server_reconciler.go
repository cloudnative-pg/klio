package controller

import (
	"context"
	"fmt"
	"maps"
	"path"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

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
	kopiaDataMountPath       = "/data"
	kopiaCacheTier1MountPath = "/cache_tier1"
	kopiaCacheTier2MountPath = "/cache_tier2"

	fileSourceBasePath     = "/files"
	tier1EncKeyFileVolName = "tier1-enc-key-file"
	tier1IdentityVolName   = "tier1-identity"
	tier2EncKeyFileVolName = "tier2-enc-key-file"
	tier2IdentityVolName   = "tier2-identity"
)

func (r *ServerReconciler) reconcile(ctx context.Context, server *kliov1alpha1.Server) (ctrl.Result, error) {
	if err := r.reconcileService(ctx, server); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile Service: %w", err)
	}

	// Reconcile PVC resizes before StatefulSet to ensure PVCs are expanded
	// before the StatefulSet is recreated. VolumeClaimTemplates only define
	// specs for new PVCs, so explicit patching is required to resize existing ones.
	if result, err := r.reconcilePVCResizes(ctx, server); err != nil || !result.IsZero() {
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to reconcile PVC resizes: %w", err)
		}

		return result, nil
	}

	return r.reconcileStatefulSet(ctx, server)
}

//nolint:cyclop
func (r *ServerReconciler) reconcileStatefulSet(
	ctx context.Context, server *kliov1alpha1.Server,
) (ctrl.Result, error) {
	contextLogger := logf.FromContext(ctx)
	klioName := server.Name + "-klio"

	pprof, _ := strconv.ParseBool(server.GetAnnotations()["klio.cnpg.io/pprof"])

	volumes := r.buildVolumes(server)
	volumeMounts := r.buildVolumeMounts(server)

	// Build container ports - always include tier1 ports, add tier2 ports if tier2 is enabled
	containerPorts := []corev1.ContainerPort{
		{Name: "base", ContainerPort: 51515, Protocol: corev1.ProtocolTCP},
		{Name: "wal", ContainerPort: 52000, Protocol: corev1.ProtocolTCP},
	}
	if server.Spec.Tier2 != nil {
		containerPorts = append(containerPorts,
			corev1.ContainerPort{Name: "tier2-base", ContainerPort: 51516, Protocol: corev1.ProtocolTCP},
			corev1.ContainerPort{Name: "tier2-wal", ContainerPort: 52001, Protocol: corev1.ProtocolTCP},
		)
	}

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
			Replicas:    ptr.To(int32(1)),
			// Explicitly retain PVCs on StatefulSet deletion and scale-down
			// to prevent data loss when the StatefulSet is recreated due to
			// immutable field changes (e.g. VolumeClaimTemplates).
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
				WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
				WhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			},
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
					Containers: []corev1.Container{
						{
							Name: "server",
							Args: []string{
								"server",
								"start",
								"--tier1=" + strconv.FormatBool(server.Spec.Tier1 != nil),
								"--tier2=" + strconv.FormatBool(server.Spec.Tier2 != nil),
							},
							Image:           server.Spec.Image,
							ImagePullPolicy: server.Spec.ImagePullPolicy,
							VolumeMounts:    volumeMounts,
							Ports:           containerPorts,
							Env:             newServerEnvBuilder(server).addCommonEnvs().addServerEnvs().build(),
						},
					},
					Volumes: volumes,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{},
	}

	if server.Spec.Tier1 != nil {
		injectTier1VolumeClaimTemplates(expected, *server)
	}

	if server.Spec.Queue != nil {
		injectQueueConfiguration(expected, *server)
	}

	// Add Tier2 containers if the server has Tier 2 configuration
	if server.Spec.Tier2 != nil {
		injectTier2VolumeClaimTemplates(expected, *server)
	}

	if server.Spec.Template != nil {
		merged, err := podtemplate.Merge(&expected.Spec.Template, server.Spec.Template.ToCoreV1())
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to merge pod templates: %w", err)
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
		return ctrl.Result{}, fmt.Errorf("failed to compute hash for Kopia configuration: %w", err)
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
			return newIncorrectOwnershipError(statefulset, server)
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
	// Approach inspired by the Prometheus operator's ForceUpdateStatefulSet:
	// when the API server rejects the update because of immutable fields
	// (e.g. VolumeClaimTemplates changed), delete the StatefulSet and let
	// the next reconciliation recreate it with the correct spec. PVCs are
	// retained because PersistentVolumeClaimRetentionPolicy is set to Retain.
	// ref: https://github.com/prometheus-operator/prometheus-operator/blob/main/pkg/k8s/statefulset.go
	if apierrors.IsInvalid(err) {
		if statefulset.CreationTimestamp.IsZero() {
			return ctrl.Result{}, err
		}

		contextLogger.Info("StatefulSet update rejected due to immutable field change, deleting for recreation",
			"reason", err.Error())

		if deleteErr := r.Delete(ctx, statefulset, &client.DeleteOptions{
			PropagationPolicy: ptr.To(metav1.DeletePropagationForeground),
		}); deleteErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete StatefulSet for recreation: %w", deleteErr)
		}

		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to reconcile statefulset (%s) %s/%s: %w",
			op,
			statefulset.Namespace,
			statefulset.Name,
			err)
	}

	return ctrl.Result{}, nil
}

func injectQueueConfiguration(expected *appsv1.StatefulSet, server kliov1alpha1.Server) {
	expected.Spec.VolumeClaimTemplates = append(expected.Spec.VolumeClaimTemplates, corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "queue",
			Labels: map[string]string{
				klioServerLabel: server.Name,
				pvcTypeLabel:    pvcTypeQueue,
			},
		},
		Spec: server.Spec.Queue.PersistentVolumeClaimTemplate,
	})
}

func enablePProf(containers []corev1.Container) {
	for i := range containers {
		// No pprof on NATS, there's no such option
		if containers[i].Name == "nats" {
			continue
		}

		containers[i].Args = append(
			containers[i].Args,
			"--pprof-server=:606"+strconv.Itoa(i))
	}
}

func injectTier1VolumeClaimTemplates(
	ss *appsv1.StatefulSet,
	server kliov1alpha1.Server,
) {
	ss.Spec.VolumeClaimTemplates = append(ss.Spec.VolumeClaimTemplates,
		corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
				Labels: map[string]string{
					klioServerLabel: server.Name,
					pvcTypeLabel:    pvcTypeData,
				},
			},
			Spec: server.Spec.Tier1.Data.PersistentVolumeClaimTemplate,
		},
		corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cachetier1",
				Labels: map[string]string{
					klioServerLabel: server.Name,
					pvcTypeLabel:    pvcTypeCacheTier1,
				},
			},
			Spec: server.Spec.Tier1.Cache.PersistentVolumeClaimTemplate,
		})
}

func injectTier2VolumeClaimTemplates(
	ss *appsv1.StatefulSet,
	server kliov1alpha1.Server,
) {
	ss.Spec.VolumeClaimTemplates = append(ss.Spec.VolumeClaimTemplates,
		corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cachetier2",
				Labels: map[string]string{
					klioServerLabel: server.Name,
					pvcTypeLabel:    pvcTypeCacheTier2,
				},
			},
			Spec: server.Spec.Tier2.Cache.PersistentVolumeClaimTemplate,
		})
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
			return newIncorrectOwnershipError(service, server)
		}

		if service.Labels == nil {
			service.Labels = map[string]string{}
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
		if server.Spec.Tier2 != nil {
			service.Spec.Ports = append(service.Spec.Ports,
				corev1.ServicePort{
					Name: "tier2-base",
					Port: 51516,
				},
				corev1.ServicePort{
					Name: "tier2-wal",
					Port: 52001,
				},
			)
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

func buildFileSourceVolMount(volName string, src kliov1alpha1.FileSource) (corev1.Volume, corev1.VolumeMount) {
	mountPath := path.Join(fileSourceBasePath, volName)

	return corev1.Volume{
			Name:         volName,
			VolumeSource: src.FileReference.Volume,
		}, corev1.VolumeMount{
			Name:      volName,
			MountPath: mountPath,
			ReadOnly:  true,
		}
}

// identityVolumeDefaultMode is the file mode for identity file volumes.
// The core refuses to start if the identity file is group/other-readable.
var identityVolumeDefaultMode = ptr.To(int32(0o400)) //nolint:gochecknoglobals // constant-like value

// buildIdentityVolMount builds a volume and mount for an identity file,
// forcing DefaultMode 0400 on volume sources that support it so the
// core's permission check passes.
func buildIdentityVolMount(volName string, src kliov1alpha1.FileSource) (corev1.Volume, corev1.VolumeMount) {
	vol, mount := buildFileSourceVolMount(volName, src)

	switch {
	case vol.Secret != nil:
		vol.Secret.DefaultMode = identityVolumeDefaultMode
	case vol.ConfigMap != nil:
		vol.ConfigMap.DefaultMode = identityVolumeDefaultMode
	case vol.Projected != nil:
		vol.Projected.DefaultMode = identityVolumeDefaultMode
	}

	return vol, mount
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
			Name: "client-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: server.Spec.ClientCASecretName,
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if server.Spec.Tier1 != nil {
		vol, _ := buildFileSourceVolMount(tier1EncKeyFileVolName, server.Spec.Tier1.EncryptionKeyFile)
		volumes = append(volumes, vol)

		vol, _ = buildIdentityVolMount(tier1IdentityVolName, server.Spec.Tier1.IdentityFile)
		volumes = append(volumes, vol)
	}

	if server.Spec.Tier2 != nil {
		vol, _ := buildFileSourceVolMount(tier2EncKeyFileVolName, server.Spec.Tier2.EncryptionKeyFile)
		volumes = append(volumes, vol)

		vol, _ = buildIdentityVolMount(tier2IdentityVolName, server.Spec.Tier2.IdentityFile)
		volumes = append(volumes, vol)

		var sources []corev1.VolumeProjection

		if server.Spec.Tier2.S3 != nil && server.Spec.Tier2.S3.CustomCABundle != nil {
			sources = append(sources, corev1.VolumeProjection{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: server.Spec.Tier2.S3.CustomCABundle.Name,
					},
					Items: []corev1.KeyToPath{
						{
							Path: "custom_ca_bundle.pem",
							Key:  server.Spec.Tier2.S3.CustomCABundle.Key,
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
			})
	}

	return volumes
}

func (r *ServerReconciler) buildVolumeMounts(server *kliov1alpha1.Server) []corev1.VolumeMount {
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
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}

	if server.Spec.Tier1 != nil {
		volumeMounts = append(
			volumeMounts,
			corev1.VolumeMount{
				Name:      "data",
				MountPath: kopiaDataMountPath,
			},
			corev1.VolumeMount{
				Name:      "cachetier1",
				MountPath: kopiaCacheTier1MountPath,
			},
		)
		_, mount := buildFileSourceVolMount(tier1EncKeyFileVolName, server.Spec.Tier1.EncryptionKeyFile)
		volumeMounts = append(volumeMounts, mount)

		_, mount = buildIdentityVolMount(tier1IdentityVolName, server.Spec.Tier1.IdentityFile)
		volumeMounts = append(volumeMounts, mount)
	}

	if server.Spec.Queue != nil {
		volumeMounts = append(
			volumeMounts,
			corev1.VolumeMount{
				Name:      "queue",
				MountPath: "/queue",
			},
		)
	}

	if server.Spec.Tier2 != nil {
		volumeMounts = append(
			volumeMounts,
			corev1.VolumeMount{
				Name:      "tier2",
				MountPath: "/tier2",
			},
			corev1.VolumeMount{
				Name:      "cachetier2",
				MountPath: kopiaCacheTier2MountPath,
			},
		)
		_, mount := buildFileSourceVolMount(tier2EncKeyFileVolName, server.Spec.Tier2.EncryptionKeyFile)
		volumeMounts = append(volumeMounts, mount)

		_, mount = buildIdentityVolMount(tier2IdentityVolName, server.Spec.Tier2.IdentityFile)
		volumeMounts = append(volumeMounts, mount)
	}

	return volumeMounts
}

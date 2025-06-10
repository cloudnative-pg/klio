package controller

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kliov1alpha1 "github.com/cloudnative-pg/klio/pkg/operator/api/v1alpha1"
)

const klioServerAnnotation = "klio.cnpg.io/klio-server"

func (r *ServerReconciler) reconcile(ctx context.Context, server *kliov1alpha1.Server) error {
	if err := r.reconcilePersistentVolumeClaim(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile PersistentVolumeClaim: %w", err)
	}

	if err := r.reconcilePod(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile Pod: %w", err)
	}

	if err := r.reconcileService(ctx, server); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	return nil
}

func (r *ServerReconciler) reconcilePersistentVolumeClaim(ctx context.Context, server *kliov1alpha1.Server) error {
	expected := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerAnnotation: server.Name,
			},
		},
		Spec: server.Spec.PersistentVolumeClaimTemplate,
	}

	if err := ctrl.SetControllerReference(server, expected, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	var current corev1.PersistentVolumeClaim
	err := r.Get(ctx, client.ObjectKeyFromObject(expected), &current)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		return r.Create(ctx, expected)
	}

	// check if its owned by us
	if !metav1.IsControlledBy(&current, server) {
		return fmt.Errorf("persistent volume claim %s/%s is not owned by Server %s/%s", current.Namespace, current.Name, server.Namespace, server.Name)
	}

	// TODO: hash
	if !reflect.DeepEqual(current.Spec, expected.Spec) {
		return r.Update(ctx, expected)
	}

	return nil
}

func (r *ServerReconciler) reconcileService(ctx context.Context, server *kliov1alpha1.Server) error {
	expected := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels: map[string]string{
				klioServerAnnotation: server.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				klioServerAnnotation: server.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "klio",
					Port:       52000,
					TargetPort: intstr.FromString("klio"),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "kopia",
					Port:       51515,
					TargetPort: intstr.FromString("kopia"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := ctrl.SetControllerReference(server, expected, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on Service: %w", err)
	}

	// Check if the service already exists
	var current corev1.Service
	err := r.Get(ctx, client.ObjectKeyFromObject(expected), &current)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		// Create service if not found
		return r.Create(ctx, expected)
	}

	// Check if it's owned by us
	if !metav1.IsControlledBy(&current, server) {
		return fmt.Errorf("service %s/%s is not owned by Server %s/%s", current.Namespace, current.Name, server.Namespace, server.Name)
	}

	// Update if spec doesn't match
	if !reflect.DeepEqual(current.Spec, expected.Spec) {
		return r.Patch(ctx, &current, client.MergeFrom(expected))
	}

	return nil
}

func (r *ServerReconciler) reconcilePod(ctx context.Context, server *kliov1alpha1.Server) error {
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
		{Name: "KOPIA_CONFIG_PATH", Value: server.Spec.KopiaConfiguration.ConfigPath},
		{Name: "KOPIA_LOG_DIR", Value: server.Spec.KopiaConfiguration.LogDirectory},
		{Name: "KOPIA_CACHE_DIRECTORY", Value: server.Spec.KopiaConfiguration.CacheDirectory},
		{Name: "USER", Value: server.Spec.KopiaConfiguration.User},
	}

	klioEnv, err := r.getKlioEnvs(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to get Klio environment variables: %w", err)
	}

	expected := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				klioServerAnnotation: server.Name,
			},
		},
		Spec: corev1.PodSpec{
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
					Name:            "kopia-initialize",
					Command:         []string{"sh", "-c", "(test ! -d /data/pgdata && kopia repository create filesystem --path=/data/pgdata) ||:"},
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
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
							corev1.ResourceCPU:    resource.MustParse("750m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
							corev1.ResourceCPU:    resource.MustParse("750m"),
						},
					},
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
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1024Mi"),
							corev1.ResourceCPU:    resource.MustParse("750m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1024Mi"),
							corev1.ResourceCPU:    resource.MustParse("750m"),
						},
					},
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
			},
		},
	}

	// TODO improve
	type hashBuilder struct {
		spec    kliov1alpha1.ServerSpec
		version int
	}

	hash, err := ComputeHash(hashBuilder{spec: server.Spec, version: 1})
	if err != nil {
		return fmt.Errorf("failed to compute hash for Kopia configuration: %w", err)
	}

	expected.ObjectMeta.Annotations = map[string]string{
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
		return fmt.Errorf("pod is not owned by Server %s/%s", server.Namespace, server.Name)
	}

	if current.Annotations["klio.cnpg.io/klio-server-hash"] != hash {
		return r.Update(ctx, expected)
	}

	return nil
}

func (r *ServerReconciler) getKlioEnvs(ctx context.Context, server *kliov1alpha1.Server) ([]corev1.EnvVar, error) {
	// Get password from secret
	var klioPasswordSecret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: server.Namespace, Name: server.Spec.Password.Name}, &klioPasswordSecret); err != nil {
		return nil, fmt.Errorf("failed to get Klio password secret: %w", err)
	}

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
	}, nil
}

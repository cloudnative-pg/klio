package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"net"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/decoder"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/object"
	"github.com/cloudnative-pg/cnpg-i/pkg/lifecycle"
	"github.com/cloudnative-pg/machinery/pkg/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LifecycleImplementation is the implementation of the lifecycle handler.
type LifecycleImplementation struct {
	lifecycle.UnimplementedOperatorLifecycleServer
	Client client.Client
}

// GetCapabilities exposes the lifecycle capabilities.
func (impl LifecycleImplementation) GetCapabilities(
	_ context.Context,
	_ *lifecycle.OperatorLifecycleCapabilitiesRequest,
) (*lifecycle.OperatorLifecycleCapabilitiesResponse, error) {
	return &lifecycle.OperatorLifecycleCapabilitiesResponse{
		LifecycleCapabilities: []*lifecycle.OperatorLifecycleCapabilities{
			{
				Group: "",
				Kind:  "Pod",
				OperationTypes: []*lifecycle.OperatorOperationType{
					{
						Type: lifecycle.OperatorOperationType_TYPE_CREATE,
					},
					{
						Type: lifecycle.OperatorOperationType_TYPE_EVALUATE,
					},
				},
			},
			{
				Group: batchv1.GroupName,
				Kind:  "Job",
				OperationTypes: []*lifecycle.OperatorOperationType{
					{
						Type: lifecycle.OperatorOperationType_TYPE_CREATE,
					},
				},
			},
		},
	}, nil
}

// LifecycleHook is called when creating Kubernetes services.
func (impl LifecycleImplementation) LifecycleHook(
	ctx context.Context,
	request *lifecycle.OperatorLifecycleRequest,
) (*lifecycle.OperatorLifecycleResponse, error) {
	contextLogger := log.FromContext(ctx).WithName("lifecycle")
	contextLogger.Info("Lifecycle hook reconciliation start")
	operation := request.GetOperationType().GetType().Enum()
	if operation == nil {
		return nil, errors.New("no operation set")
	}

	var cluster cnpgv1.Cluster
	if err := decoder.DecodeObjectLenient(
		request.GetClusterDefinition(),
		&cluster,
	); err != nil {
		return nil, err
	}

	conf, err := newConfigFromCluster(&cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to create config from cluster definition")
		return nil, fmt.Errorf("error creating config from cluster definition: %w", err)
	}

	kind, err := object.GetKind(request.GetObjectDefinition())
	if err != nil {
		return nil, err
	}

	switch kind {
	case "Pod":
		contextLogger.Info("Reconciling pod")
		return impl.reconcilePod(ctx, &cluster, request, conf)
	case "Job":
		// TODO: implement job reconciliation
		contextLogger.Info("Reconciling job")
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}

func (impl LifecycleImplementation) reconcilePod(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
	request *lifecycle.OperatorLifecycleRequest,
	cfg *pluginConfiguration,
) (*lifecycle.OperatorLifecycleResponse, error) {
	pod, err := decoder.DecodePodJSON(request.GetObjectDefinition())
	if err != nil {
		return nil, err
	}

	contextLogger := log.FromContext(ctx).WithName("klio-pod-lifecycle").
		WithValues("podName", pod.Name)

	mutatedPod := pod.DeepCopy()

	if err := reconcilePodSpec(
		cluster,
		&mutatedPod.Spec,
		cfg); err != nil {
		contextLogger.Error(err, "Failed to reconcile pod spec")
		return nil, fmt.Errorf("failed to reconcile pod spec: %w", err)
	}

	patch, err := object.CreatePatch(mutatedPod, pod)
	if err != nil {
		return nil, err
	}

	contextLogger.Debug("generated patch", "content", string(patch))

	return &lifecycle.OperatorLifecycleResponse{
		JsonPatch: patch,
	}, nil
}

func reconcilePodSpec(cluster *cnpgv1.Cluster, spec *corev1.PodSpec, cfg *pluginConfiguration) error {
	const mainContainerName = "postgres"

	spec.Volumes = ensureVolume(spec.Volumes, corev1.Volume{
		Name: "klio-server-tls",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: cfg.ServerSecretName,
			},
		},
	})

	sidecarTemplate := corev1.Container{
		Image:           "registry.dev:5000/klio-testing:dev",
		ImagePullPolicy: corev1.PullAlways, // TODO: this should be a plugin configuration parameter
		RestartPolicy:   ptr.To(corev1.ContainerRestartPolicyAlways),
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  ptr.To(int64(26)),
			RunAsGroup: ptr.To(int64(26)),
		},
		Env: []corev1.EnvVar{
			{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
			{Name: "PGHOST", Value: "/controller/run"},
			{Name: "PGPORT", Value: "5432"},
			{Name: "SOURCE_DSN", Value: "user=postgres replication=yes application_name=klio"},
			{Name: "SOURCE_STANDARD_DSN", Value: "user=postgres application_name=klio"},
			{Name: "SOURCE_SLOT", Value: "klio"},
			{Name: "CLIENT_BASE_URL", Value: "https://" + net.JoinHostPort(cfg.ServerAddress, "51515")},
			{Name: "CLIENT_BASE_SERVER_CERT_PATH", Value: "/certs/tls.crt"},
			{Name: "CLIENT_BASE_HOSTNAME", Value: cluster.Name},
			{Name: "CLIENT_BASE_USERNAME", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ClientSecretName,
					},
					Key: corev1.BasicAuthUsernameKey,
				},
			}},
			{Name: "CLIENT_BASE_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ClientSecretName,
					},
					Key: corev1.BasicAuthPasswordKey,
				},
			}},
			{Name: "CLIENT_WAL_ADDRESS", Value: net.JoinHostPort(cfg.ServerAddress, "52000")},
			{Name: "CLIENT_WAL_CLUSTER_NAME", Value: cluster.Name},
			{Name: "CLIENT_WAL_SERVER_CERT_PATH", Value: "/certs/tls.crt"},
			{Name: "CLIENT_WAL_USERNAME", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ClientSecretName,
					},
					Key: corev1.BasicAuthUsernameKey,
				},
			}},
			{Name: "CLIENT_WAL_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ClientSecretName,
					},
					Key: corev1.BasicAuthPasswordKey,
				},
			}},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "klio-server-tls", MountPath: "/certs"},
			{Name: "pgdata", MountPath: "/var/lib/postgresql/data"},
			{Name: "scratch-data", MountPath: "/controller"},
		},
	}

	// merge the main container envs if they aren't already set
	for _, container := range spec.Containers {
		if container.Name == mainContainerName {
			for _, env := range container.Env {
				found := false
				for _, existingEnv := range sidecarTemplate.Env {
					if existingEnv.Name == env.Name {
						found = true
						break
					}
				}
				if !found {
					sidecarTemplate.Env = append(sidecarTemplate.Env, env)
				}
			}

			break
		}
	}

	sendWalSidecar := sidecarTemplate.DeepCopy()
	sendWalSidecar.Name = "klio-wal"
	sendWalSidecar.Args = []string{"send-wal"}

	pluginSidecar := sidecarTemplate.DeepCopy()
	pluginSidecar.Name = "klio-plugin"
	pluginSidecar.Args = []string{"cnpgi"}

	if err := injectPluginSidecarPodSpec(spec, sendWalSidecar, mainContainerName); err != nil {
		return err
	}

	if err := injectPluginSidecarPodSpec(spec, pluginSidecar, mainContainerName); err != nil {
		return err
	}

	return nil
}

// TODO: move to machinery once the logic is finalized

// InjectPluginVolumePodSpec injects the plugin volume into a CNPG Pod spec.
func InjectPluginVolumePodSpec(spec *corev1.PodSpec, mainContainerName string) {
	const (
		pluginVolumeName = "plugins"
		pluginMountPath  = "/plugins"
	)

	foundPluginVolume := false
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == pluginVolumeName {
			foundPluginVolume = true
		}
	}

	if foundPluginVolume {
		return
	}

	spec.Volumes = ensureVolume(spec.Volumes, corev1.Volume{
		Name: pluginVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	for i := range spec.Containers {
		if spec.Containers[i].Name == mainContainerName {
			spec.Containers[i].VolumeMounts = ensureVolumeMount(
				spec.Containers[i].VolumeMounts,
				corev1.VolumeMount{
					Name:      pluginVolumeName,
					MountPath: pluginMountPath,
				},
			)
		}
	}
}

// injectPluginSidecarPodSpec injects a plugin sidecar into a CNPG Pod spec.
func injectPluginSidecarPodSpec(
	spec *corev1.PodSpec,
	sidecar *corev1.Container,
	mainContainerName string,
) error {
	sidecar = sidecar.DeepCopy()
	InjectPluginVolumePodSpec(spec, mainContainerName)

	sidecarContainerFound := false
	mainContainerFound := false
	for i := range spec.Containers {
		if spec.Containers[i].Name == mainContainerName {
			sidecar.VolumeMounts = ensureVolumeMount(sidecar.VolumeMounts, spec.Containers[i].VolumeMounts...)
			mainContainerFound = true
		}
	}

	if !mainContainerFound {
		return errors.New("main container not found")
	}

	for i := range spec.InitContainers {
		if spec.InitContainers[i].Name == sidecar.Name {
			sidecarContainerFound = true
			spec.InitContainers[i] = *sidecar
		}
	}

	if !sidecarContainerFound {
		spec.InitContainers = append(spec.InitContainers, *sidecar)
	}

	return nil
}

// ensureVolume makes sure the passed volume is present in the list of volumes.
// If the volume is already present, it is updated.
func ensureVolume(volumes []corev1.Volume, volume corev1.Volume) []corev1.Volume {
	volumeFound := false
	for i := range volumes {
		if volumes[i].Name == volume.Name {
			volumeFound = true
			volumes[i] = volume
		}
	}

	if !volumeFound {
		volumes = append(volumes, volume)
	}

	return volumes
}

// ensureVolumeMount makes sure the passed volume mounts are present in the list of volume mounts.
// If a volume mount is already present, it is updated.
func ensureVolumeMount(mounts []corev1.VolumeMount, volumeMounts ...corev1.VolumeMount) []corev1.VolumeMount {
	for _, mount := range volumeMounts {
		mountFound := false
		for i := range mounts {
			if mounts[i].Name == mount.Name {
				mountFound = true
				mounts[i] = mount
				break
			}
		}

		if !mountFound {
			mounts = append(mounts, mount)
		}
	}

	return mounts
}

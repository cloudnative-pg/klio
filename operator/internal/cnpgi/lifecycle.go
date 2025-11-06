package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/decoder"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/object"
	"github.com/cloudnative-pg/cnpg-i/pkg/lifecycle"
	"github.com/cloudnative-pg/machinery/pkg/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/pkg/config"
)

const (
	pgdata                     = "/var/lib/postgresql/data/pgdata"
	klioConfigSecretNameSuffix = "-klio-config" //nolint:gosec
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

	kind, err := object.GetKind(request.GetObjectDefinition())
	if err != nil {
		return nil, err
	}

	switch kind {
	case "Pod":
		contextLogger.Info("Reconciling pod")
		return impl.reconcilePod(ctx, &cluster, request)
	case "Job":
		return impl.reconcileJob(ctx, &cluster, request)
	default:
		return nil, fmt.Errorf("unsupported kind: %s", kind)
	}
}

//nolint:cyclop
func (impl LifecycleImplementation) reconcileJob(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
	request *lifecycle.OperatorLifecycleRequest,
) (*lifecycle.OperatorLifecycleResponse, error) {
	contextLogger := log.FromContext(ctx).WithName("klio-job-lifecycle")
	klioConfigs, _, err := klioConfigsFromCluster(ctx, impl.Client, cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to get klio configuration from cluster definition")
		return nil, err
	}

	recoveryPluginConfig := cluster.GetRecoverySourcePlugin()
	if recoveryPluginConfig == nil ||
		recoveryPluginConfig.Name != PluginName ||
		!recoveryPluginConfig.IsEnabled() {
		// not our plugin, skip
		return nil, nil
	}

	if recoveryPluginConfig.Parameters[PluginConfigurationRefParam] == "" {
		contextLogger.Warning("recovery plugin configuration missing 'ref' parameter")
		return nil, errors.New("recovery plugin configuration missing 'ref' parameter")
	}

	clusterPC := &kliov1alpha1.PluginConfiguration{}
	err = impl.Client.Get(ctx,
		client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      recoveryPluginConfig.Parameters[PluginConfigurationRefParam],
		},
		clusterPC)
	if err != nil {
		contextLogger.Error(err, "failed to get client configuration")
		return nil, fmt.Errorf("failed to get client configuration: %w", err)
	}

	if clusterPC.Spec.BackupID == "" {
		contextLogger.Error(errors.New("no backupID specified"), "cannot find backupID")
		return nil, errors.New("no backupID specified")
	}
	backupID := clusterPC.Spec.BackupID

	// Reconcile the configuration secret
	if err := impl.reconcileKlioConfigSecret(ctx, klioConfigs, cluster); err != nil {
		return nil, err
	}

	var job batchv1.Job
	if err := decoder.DecodeObjectStrict(
		request.GetObjectDefinition(),
		&job,
		batchv1.SchemeGroupVersion.WithKind("Job"),
	); err != nil {
		contextLogger.Error(err, "failed to decode job")
		return nil, err
	}

	contextLogger = log.FromContext(ctx).WithName("klio-job-lifecycle").
		WithValues("jobName", job.Name)
	contextLogger.Debug("starting job reconciliation")

	jobRole := getCNPGJobRole(&job)
	if jobRole != "full-recovery" &&
		jobRole != "snapshot-recovery" {
		contextLogger.Debug("job is not a recovery job, skipping")
		return nil, nil
	}

	mutatedJob := job.DeepCopy()

	sidecarsToEnrich := []corev1.Container{
		{
			Name: "klio-restore",
			Args: []string{
				"cnpgi",
				"restore",
				backupID,
				pgdata,
			},
			Env: []corev1.EnvVar{
				{Name: "CONTAINER_NAME", Value: "klio-restore"},
			},
		},
	}

	if err := reconcilePodSpec(
		ctx,
		impl.Client,
		cluster,
		&mutatedJob.Spec.Template.Spec,
		jobRole,
		reconcilePodSpecConfiguration{
			sidecarsToEnrich: sidecarsToEnrich,
		}); err != nil {
		return nil, fmt.Errorf("while reconciling pod spec for job: %w", err)
	}

	patch, err := object.CreatePatch(mutatedJob, &job)
	if err != nil {
		return nil, err
	}

	contextLogger.Debug("generated patch", "content", string(patch))

	return &lifecycle.OperatorLifecycleResponse{
		JsonPatch: patch,
	}, nil
}

// reconcileKlioConfigSecret reconciles the Klio configuration secret containing all the Klio configurations
// files for the cluster and the external clusters.
func (impl LifecycleImplementation) reconcileKlioConfigSecret(
	ctx context.Context,
	klioConfigs map[string]*config.Data,
	cluster *cnpgv1.Cluster,
) error {
	contextLogger := log.FromContext(ctx).WithName("klio-config-secret")

	generatedSecret, err := klioConfigsToSecret(klioConfigs, cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to generate configuration secret from cluster definition")
		return fmt.Errorf("error generating configuration secret from cluster definition: %w", err)
	}

	originalSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatedSecret.Name,
			Namespace: generatedSecret.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(cluster, &originalSecret, impl.Client.Scheme()); err != nil {
		return fmt.Errorf(
			"failed to update controller reference on Klio configuration secret %q: %w",
			generatedSecret.Name,
			err,
		)
	}

	if _, err := controllerutil.CreateOrUpdate(
		ctx,
		impl.Client,
		&originalSecret,
		func() error {
			originalSecret.Data = generatedSecret.Data
			return nil
		},
	); err != nil {
		return fmt.Errorf("failed to update or create configuration secret: %w", err)
	}

	return nil
}

func (impl LifecycleImplementation) reconcilePod(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
	request *lifecycle.OperatorLifecycleRequest,
) (*lifecycle.OperatorLifecycleResponse, error) {
	contextLogger := log.FromContext(ctx).WithName("klio-pod-lifecycle")

	klioConfigs, clusterPC, err := klioConfigsFromCluster(ctx, impl.Client, cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to get klio configuration from cluster definition")
		return nil, err
	}

	// Reconcile the configuration secret
	if err := impl.reconcileKlioConfigSecret(ctx, klioConfigs, cluster); err != nil {
		return nil, err
	}

	pod, err := decoder.DecodePodJSON(request.GetObjectDefinition())
	if err != nil {
		return nil, err
	}

	contextLogger = log.FromContext(ctx).WithName("klio-pod-lifecycle").
		WithValues("podName", pod.Name)

	mutatedPod := pod.DeepCopy()

	sidecarsToEnrich := []corev1.Container{
		buildInstanceSidecarTemplate(),
		buildSendWALSidecarTemplate(pod, cluster, clusterPC),
	}

	if err := reconcilePodSpec(
		ctx,
		impl.Client,
		cluster,
		&mutatedPod.Spec,
		"postgres",
		reconcilePodSpecConfiguration{
			sidecarsToEnrich: sidecarsToEnrich,
		}); err != nil {
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

func buildSendWALSidecarTemplate(
	pod *corev1.Pod,
	cluster *cnpgv1.Cluster,
	clusterPC *kliov1alpha1.PluginConfiguration,
) corev1.Container {
	if clusterPC == nil {
		return corev1.Container{}
	}

	args := []string{
		"cnpgi",
		"send-wal",
		"--config", "/var/lib/postgresql/klio/" + klioArchiveConfigKey,
		"--pod-name", pod.Name,
		"--cluster-name", cluster.Name,
		"--cluster-namespace", cluster.Namespace,
	}

	return corev1.Container{
		Name: "klio-wal",
		Args: args,
		Env: []corev1.EnvVar{
			{Name: "CONTAINER_NAME", Value: "klio-wal"},
		},
	}
}

func buildInstanceSidecarTemplate() corev1.Container {
	return corev1.Container{
		Name: "klio-plugin",
		Args: []string{"cnpgi", "instance"},
		Env: []corev1.EnvVar{
			{Name: "CONTAINER_NAME", Value: "klio-plugin"},
		},
	}
}

type reconcilePodSpecConfiguration struct {
	sidecarsToEnrich []corev1.Container
	enablePPROF      bool
}

// reconcilePodSpec reconciles the pod spec to include the klio server sidecar and its configuration.
//
//nolint:cyclop
func reconcilePodSpec(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
	spec *corev1.PodSpec,
	mainContainerName string,
	cfg reconcilePodSpecConfiguration,
) error {
	// Add PostgreSQL data volume and mounts
	volumeMounts := []corev1.VolumeMount{
		{Name: "pgdata", MountPath: "/var/lib/postgresql/data"},
		{Name: "scratch-data", MountPath: "/controller"},
	}

	// Add the Klio configuration volume and mount
	spec.Volumes = ensureVolume(spec.Volumes, corev1.Volume{
		Name: "klio-config",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: cluster.Name + klioConfigSecretNameSuffix,
			},
		},
	})
	volumeMounts = ensureVolumeMount(volumeMounts, corev1.VolumeMount{
		Name:      "klio-config",
		MountPath: "/var/lib/postgresql/klio",
	})

	// Add the TLS volumes and mounts for the server and client secrets
	tlsVolumesAndMounts, err := getTLSVolumesAndMounts(ctx, cli, cluster)
	if err != nil {
		return fmt.Errorf("failed to get TLS volumes and mounts: %w", err)
	}
	for _, v := range tlsVolumesAndMounts {
		spec.Volumes = ensureVolume(spec.Volumes, v.Volume)
		volumeMounts = ensureVolumeMount(volumeMounts, v.VolumeMount)
	}

	sidecarTemplate := corev1.Container{
		Image:           os.Getenv("SIDECAR_IMAGE"),
		RestartPolicy:   ptr.To(corev1.ContainerRestartPolicyAlways),
		ImagePullPolicy: cluster.Spec.ImagePullPolicy,
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  ptr.To(int64(26)),
			RunAsGroup: ptr.To(int64(26)),
		},
		Env: []corev1.EnvVar{
			{Name: "PGDATA", Value: pgdata},
			{Name: "PGHOST", Value: "/controller/run"},
			{Name: "PGPORT", Value: "5432"},
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
			{
				Name:  "CUSTOM_CNPG_GROUP",
				Value: cluster.GetObjectKind().GroupVersionKind().Group,
			},
			{
				Name:  "CUSTOM_CNPG_VERSION",
				Value: cluster.GetObjectKind().GroupVersionKind().Version,
			},
		},
		VolumeMounts: volumeMounts,
	}

	var mainContainer *corev1.Container
	for i := range spec.Containers {
		if spec.Containers[i].Name == mainContainerName {
			mainContainer = &spec.Containers[i]
			break
		}
	}
	if mainContainer == nil {
		return fmt.Errorf("main container %s not found in pod spec", mainContainerName)
	}

	// merge the main container envs if they aren't already set
	mergeEnvironmentVariables(*mainContainer, &sidecarTemplate)

	// If any plugin configuration has the enablePPROF parameter set to true, enable PPROF in the sidecars.
	configurations, err := getConfigurations(ctx, cli, cluster)
	if err != nil {
		return fmt.Errorf("failed to get plugin configurations: %w", err)
	}
	for _, v := range configurations {
		// consider only enabled plugin configurations for PPROF
		if v.cnpgPluginConfiguration == nil || !v.cnpgPluginConfiguration.IsEnabled() || v.klioPluginConfiguration == nil {
			continue
		}

		if v.klioPluginConfiguration.Spec.Pprof {
			cfg.enablePPROF = true
		}
	}

	for idx, baseSidecar := range cfg.sidecarsToEnrich {
		if baseSidecar.Name == "" {
			continue
		}

		sidecar := sidecarTemplate.DeepCopy()
		sidecar.Name = baseSidecar.Name
		sidecar.Args = baseSidecar.Args
		if cfg.enablePPROF {
			sidecar.Args = append(sidecar.Args, fmt.Sprintf("--pprof-server=:609%d", idx))
		}
		mergeEnvironmentVariables(baseSidecar, sidecar)
		if err := injectPluginSidecarPodSpec(spec, sidecar, mainContainerName); err != nil {
			return fmt.Errorf("failed to inject sidecar %s: %w", sidecar.Name, err)
		}
	}

	return nil
}

// mergeEnvironmentVariables ensures that the environment variables from the giver container
// are added to the receiver container, without duplicating existing variables in the receiver.
// This is useful for combining environment variables from multiple sources.
func mergeEnvironmentVariables(giver corev1.Container, receiver *corev1.Container) {
	for _, env := range giver.Env {
		found := false
		for _, existingEnv := range receiver.Env {
			if existingEnv.Name == env.Name {
				found = true
				break
			}
		}
		if !found {
			receiver.Env = append(receiver.Env, env)
		}
	}
}

type volumeAndMount struct {
	Volume      corev1.Volume
	VolumeMount corev1.VolumeMount
}

func getTLSVolumesAndMounts(
	ctx context.Context,
	cli client.Client,
	cluster *cnpgv1.Cluster,
) ([]volumeAndMount, error) {
	var volumesAndMounts []volumeAndMount

	configurations, err := getConfigurations(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin configurations: %w", err)
	}
	for hostName, conf := range configurations {
		// consider only enabled plugin configurations to avoid errors from missing parameters
		if conf == nil || conf.cnpgPluginConfiguration == nil ||
			!conf.cnpgPluginConfiguration.IsEnabled() || conf.klioPluginConfiguration == nil {
			continue
		}

		// Extract the server secret name from the klio pluginConfig
		// Assuming we can get it from the WAL repository pluginConfig or base pluginConfig
		serverSecretName := conf.klioPluginConfiguration.Spec.ServerSecretName
		if serverSecretName != "" {
			volume := corev1.Volume{
				Name: getServerSecretVolumeName(hostName),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: serverSecretName,
					},
				},
			}

			volumeMount := corev1.VolumeMount{
				Name:      getServerSecretVolumeName(hostName),
				MountPath: getServerSecretVolumeMountPath(hostName),
			}

			volumesAndMounts = append(volumesAndMounts, volumeAndMount{
				Volume:      volume,
				VolumeMount: volumeMount,
			})
		}

		clientSecretName := conf.klioPluginConfiguration.Spec.ClientSecretName
		if clientSecretName != "" {
			volume := corev1.Volume{
				Name: getClientSecretVolumeName(hostName),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: clientSecretName,
					},
				},
			}

			volumeMount := corev1.VolumeMount{
				Name:      getClientSecretVolumeName(hostName),
				MountPath: getClientSecretVolumeMountPath(hostName),
			}

			volumesAndMounts = append(volumesAndMounts, volumeAndMount{
				Volume:      volume,
				VolumeMount: volumeMount,
			})
		}
	}

	return volumesAndMounts, nil
}

func getServerSecretVolumeName(hostName string) string {
	return hostName + "-server-certs"
}

func getServerSecretVolumeMountPath(hostName string) string {
	return "/server-certs/" + hostName
}

func getClientSecretVolumeName(hostName string) string {
	return hostName + "-client-certs"
}

func getClientSecretVolumeMountPath(hostName string) string {
	return "/client-certs/" + hostName
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

// getCNPGJobRole gets the role associated to a CNPG job.
func getCNPGJobRole(job *batchv1.Job) string {
	const jobRoleLabelSuffix = "/jobRole"
	for k, v := range job.Spec.Template.Labels {
		if strings.HasSuffix(k, jobRoleLabelSuffix) {
			return v
		}
	}

	return ""
}

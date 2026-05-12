package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/decoder"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/object"
	"github.com/cloudnative-pg/cnpg-i/pkg/lifecycle"
	"github.com/cloudnative-pg/machinery/pkg/log"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

const pgdata = "/var/lib/postgresql/data/pgdata"

// KlioPluginContainerName is the name of the Klio plugin sidecar container
// that handles backup creation and management in PostgreSQL pods.
const KlioPluginContainerName = "klio-plugin"

// LifecycleImplementation is the implementation of the lifecycle handler.
type LifecycleImplementation struct {
	lifecycle.UnimplementedOperatorLifecycleServer

	Client                         client.Client
	HaveSecurityContextConstraints bool
}

// cnpgGroupVersion returns the API group and version of the cluster as
// declared in its TypeMeta, falling back to the upstream defaults when the
// wire payload omits them.
func cnpgGroupVersion(cluster *cnpgv1.Cluster) (string, string) {
	gvk := cluster.GroupVersionKind()
	group, version := gvk.Group, gvk.Version
	if group == "" {
		group = cnpgv1.SchemeGroupVersion.Group
	}
	if version == "" {
		version = cnpgv1.SchemeGroupVersion.Version
	}

	return group, version
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
		contextLogger.Warning("Operation type is not set in request")
		return nil, errors.New("no operation set")
	}

	var cluster cnpgv1.Cluster
	if err := decoder.DecodeObjectLenient(
		request.GetClusterDefinition(),
		&cluster,
	); err != nil {
		contextLogger.Error(err, "Failed to decode cluster definition")
		return nil, err
	}

	kind, err := object.GetKind(request.GetObjectDefinition())
	if err != nil {
		contextLogger.Error(err, "Failed to get object kind from definition")
		return nil, err
	}

	switch kind {
	case "Pod":
		contextLogger.Info("Reconciling pod")
		return impl.reconcilePod(ctx, &cluster, request)
	case "Job":
		contextLogger.Info("Reconciling job")
		return impl.reconcileJob(ctx, &cluster, request)
	default:
		contextLogger.Warning("Unsupported kind", "kind", kind)
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
	plugins, err := klioconfig.ResolveClusterPlugins(ctx, impl.Client, cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to get klio configuration from cluster definition")
		return nil, err
	}

	recoveryPluginConfig := cluster.GetRecoverySourcePlugin()
	if recoveryPluginConfig == nil ||
		recoveryPluginConfig.Name != klioconfig.PluginName ||
		!recoveryPluginConfig.IsEnabled() {
		// not our plugin, skip
		return nil, nil
	}

	if recoveryPluginConfig.Parameters[klioconfig.PluginConfigurationRefParam] == "" {
		contextLogger.Warning("recovery plugin configuration missing 'ref' parameter")
		return nil, errors.New("recovery plugin configuration missing 'ref' parameter")
	}

	clusterPC := &kliov1alpha1.PluginConfiguration{}
	err = impl.Client.Get(ctx,
		client.ObjectKey{
			Namespace: cluster.Namespace,
			Name:      recoveryPluginConfig.Parameters[klioconfig.PluginConfigurationRefParam],
		},
		clusterPC)
	if err != nil {
		contextLogger.Error(err, "Failed to get client configuration")
		return nil, fmt.Errorf("failed to get client configuration: %w", err)
	}

	var job batchv1.Job
	if err := decoder.DecodeObjectStrict(
		request.GetObjectDefinition(),
		&job,
		batchv1.SchemeGroupVersion.WithKind("Job"),
	); err != nil {
		contextLogger.Error(err, "Failed to decode job")
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

	// Resolve the config key for the recovery source.
	recoverySource := cluster.Spec.Bootstrap.Recovery.Source
	recoveryExternalCluster, _ := cluster.ExternalCluster(recoverySource)
	configKey := recoveryExternalCluster.GetServerName()

	// Build the restore sidecar with merge strategy:
	// 1. Start from user customization if present (as the base)
	// 2. Apply Klio required values (name, args, essential env vars)
	// 3. Template defaults will be merged later in reconcilePodSpec
	restoreSidecar := findUserContainer("klio-restore", clusterPC.Spec.Containers)
	restoreSidecar.Args = []string{
		"cnpgi",
		"restore",
		"--config", "/var/lib/postgresql/klio/" + configKey,
		pgdata,
	}
	restoreSidecar.Env = ensureEnvVar(restoreSidecar.Env, corev1.EnvVar{
		Name:  "CONTAINER_NAME",
		Value: "klio-restore",
	})

	sidecarsToEnrich := []corev1.Container{restoreSidecar}

	cnpgGroup, cnpgVersion := cnpgGroupVersion(cluster)
	if err := reconcilePodSpec(
		cluster,
		&mutatedJob.Spec.Template.Spec,
		jobRole,
		reconcilePodSpecConfiguration{
			sidecarsToEnrich: sidecarsToEnrich,
			cnpgGroup:        cnpgGroup,
			cnpgVersion:      cnpgVersion,
			plugins:          plugins,
			haveSCC:          impl.HaveSecurityContextConstraints,
		},
	); err != nil {
		contextLogger.Error(err, "Failed to reconcile pod spec for job")
		return nil, fmt.Errorf("while reconciling pod spec for job: %w", err)
	}

	patch, err := object.CreatePatch(mutatedJob, &job)
	if err != nil {
		contextLogger.Error(err, "Failed to create patch for job")
		return nil, err
	}

	contextLogger.Debug("generated patch", "content", string(patch))

	return &lifecycle.OperatorLifecycleResponse{
		JsonPatch: patch,
	}, nil
}

func (impl LifecycleImplementation) reconcilePod(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
	request *lifecycle.OperatorLifecycleRequest,
) (*lifecycle.OperatorLifecycleResponse, error) {
	contextLogger := log.FromContext(ctx).WithName("klio-pod-lifecycle")

	plugins, err := klioconfig.ResolveClusterPlugins(ctx, impl.Client, cluster)
	if err != nil {
		contextLogger.Error(err, "Failed to get klio configuration from cluster definition")
		return nil, err
	}

	pod, err := decoder.DecodePodJSON(request.GetObjectDefinition())
	if err != nil {
		contextLogger.Error(err, "Failed to decode pod definition")
		return nil, err
	}

	contextLogger = log.FromContext(ctx).WithName("klio-pod-lifecycle").
		WithValues("podName", pod.Name)

	archiveConfigKey := klioconfig.ArchiveConfigKey
	targetPC, ok := plugins[klioconfig.ArchiveConfigKey]
	if !ok {
		// No archive plugin. The only case where the instance sidecar is
		// still needed is the designated primary of a replica cluster,
		// which restores WALs from the external source.
		if !cluster.IsReplica() ||
			cluster.Status.TargetPrimary != pod.Name {
			return nil, nil
		}

		replicaSource := cluster.Spec.ReplicaCluster.Source
		ext, _ := cluster.ExternalCluster(replicaSource)
		archiveConfigKey = ""
		targetPC, ok = plugins[ext.GetServerName()]
		if !ok {
			// The cluster may be replicating using a different plugin
			return nil, nil
		}
	}

	mutatedPod := pod.DeepCopy()

	sidecarsToEnrich := []corev1.Container{
		buildInstanceSidecarTemplate(pod, cluster, targetPC, archiveConfigKey),
	}

	cnpgGroup, cnpgVersion := cnpgGroupVersion(cluster)
	if err := reconcilePodSpec(
		cluster,
		&mutatedPod.Spec,
		"postgres",
		reconcilePodSpecConfiguration{
			sidecarsToEnrich: sidecarsToEnrich,
			cnpgGroup:        cnpgGroup,
			cnpgVersion:      cnpgVersion,
			plugins:          plugins,
			haveSCC:          impl.HaveSecurityContextConstraints,
		}); err != nil {
		contextLogger.Error(err, "Failed to reconcile pod spec")
		return nil, fmt.Errorf("failed to reconcile pod spec: %w", err)
	}

	patch, err := object.CreatePatch(mutatedPod, pod)
	if err != nil {
		contextLogger.Error(err, "Failed to create patch for pod")
		return nil, err
	}

	contextLogger.Debug("generated patch", "content", string(patch))

	return &lifecycle.OperatorLifecycleResponse{
		JsonPatch: patch,
	}, nil
}

func buildInstanceSidecarTemplate(
	pod *corev1.Pod,
	cluster *cnpgv1.Cluster,
	clusterPC *kliov1alpha1.PluginConfiguration,
	archiveConfigKey string,
) corev1.Container {
	// Merge strategy:
	// 1. Start from user customization if present (as the base)
	// 2. Apply Klio required values (name, args, essential env vars)
	// 3. Template defaults will be merged later in reconcilePodSpec
	sidecar := corev1.Container{Name: KlioPluginContainerName}

	args := []string{
		"cnpgi",
		"instance",
		"--pod-name", pod.Name,
		"--cluster-name", cluster.Name,
		"--cluster-namespace", cluster.Namespace,
	}

	if archiveConfigKey != "" {
		args = append(args, "--config", path.Join("/var/lib/postgresql/klio/", archiveConfigKey))
	}

	if clusterPC != nil {
		sidecar = findUserContainer(KlioPluginContainerName, clusterPC.Spec.Containers)
	}

	sidecar.Args = args
	sidecar.Env = ensureEnvVar(sidecar.Env, corev1.EnvVar{
		Name:  "CONTAINER_NAME",
		Value: KlioPluginContainerName,
	})

	return sidecar
}

type reconcilePodSpecConfiguration struct {
	sidecarsToEnrich []corev1.Container
	plugins          klioconfig.ClusterPlugins
	cnpgGroup        string
	cnpgVersion      string
	haveSCC          bool
}

// reconcilePodSpec reconciles the pod spec to include the klio server sidecar and its configuration.
//
//nolint:cyclop
func reconcilePodSpec( // NOSONAR
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

	// Build a projected volume that maps each PC's config.yaml to the
	// configKey path the core sidecar expects (e.g. klio-archive, source-server).
	configKeys := make([]string, 0, len(cfg.plugins))
	for k := range cfg.plugins {
		configKeys = append(configKeys, k)
	}
	sort.Strings(configKeys)

	sources := make([]corev1.VolumeProjection, 0, len(cfg.plugins))
	for _, configKey := range configKeys {
		pc := cfg.plugins[configKey]
		sources = append(sources, corev1.VolumeProjection{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pc.Name,
				},
				Items: []corev1.KeyToPath{
					{Key: klioconfig.ConfigDataKey, Path: configKey},
				},
			},
		})
	}
	spec.Volumes = ensureVolume(spec.Volumes, corev1.Volume{
		Name: "klio-config",
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				Sources: sources,
			},
		},
	})
	volumeMounts = ensureVolumeMount(volumeMounts, corev1.VolumeMount{
		Name:      "klio-config",
		MountPath: "/var/lib/postgresql/klio",
	})

	// Add the TLS volumes and mounts for the server and client secrets
	tlsVolumesAndMounts := getTLSVolumesAndMounts(cfg.plugins)
	for _, v := range tlsVolumesAndMounts {
		spec.Volumes = ensureVolume(spec.Volumes, v.Volume)
		volumeMounts = ensureVolumeMount(volumeMounts, v.VolumeMount)
	}

	sidecarTemplate := corev1.Container{
		Image:           os.Getenv("SIDECAR_IMAGE"),
		RestartPolicy:   new(corev1.ContainerRestartPolicyAlways),
		ImagePullPolicy: cluster.Spec.ImagePullPolicy,
		SecurityContext: sidecarSecurityContext(cfg.haveSCC),
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
				Value: cfg.cnpgGroup,
			},
			{
				Name:  "CUSTOM_CNPG_VERSION",
				Value: cfg.cnpgVersion,
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

	// Derive pprof flag from resolved plugins.
	enablePPROF := false
	for _, pc := range cfg.plugins {
		if pc.Spec.Pprof {
			enablePPROF = true
		}
	}

	for idx, baseSidecar := range cfg.sidecarsToEnrich {
		if baseSidecar.Name == "" {
			continue
		}

		// Start from baseSidecar which already has user customizations + Klio requirements
		sidecar := baseSidecar.DeepCopy()

		// Apply PPROF if enabled
		if enablePPROF {
			sidecar.Args = append(sidecar.Args, fmt.Sprintf("--pprof-server=:609%d", idx))
		}

		// Apply sidecarTemplate defaults only if not already set by user
		if sidecar.Image == "" {
			sidecar.Image = sidecarTemplate.Image
		}
		if sidecar.ImagePullPolicy == "" {
			sidecar.ImagePullPolicy = sidecarTemplate.ImagePullPolicy
		}
		if sidecar.RestartPolicy == nil {
			sidecar.RestartPolicy = sidecarTemplate.RestartPolicy
		}
		if sidecar.SecurityContext == nil {
			sidecar.SecurityContext = sidecarTemplate.SecurityContext.DeepCopy()
		}

		// Merge environment variables and volume mounts from sidecarTemplate (only add if not present)
		mergeEnvironmentVariables(sidecarTemplate, sidecar)
		mergeVolumeMounts(sidecarTemplate, sidecar)

		if err := injectPluginSidecarPodSpec(spec, sidecar, mainContainerName); err != nil {
			return fmt.Errorf("failed to inject sidecar %s: %w", sidecar.Name, err)
		}
	}

	return nil
}

// sidecarSecurityContext returns the SecurityContext for the Klio plugin
// sidecar container. On OpenShift it returns nil so that the restricted SCC
// can assign a namespace-allocated UID.
func sidecarSecurityContext(haveSCC bool) *corev1.SecurityContext {
	if haveSCC {
		return nil
	}

	return &corev1.SecurityContext{
		RunAsUser:  new(int64(26)),
		RunAsGroup: new(int64(26)),
	}
}

// mergeEnvironmentVariables merges environment variables from the giver container into the receiver container.
// Variables already present in the receiver are preserved (receiver takes precedence).
// New variables from the giver are appended with deep copy to prevent aliasing.
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
			receiver.Env = append(receiver.Env, *env.DeepCopy())
		}
	}
}

// mergeVolumeMounts merges volume mounts from the giver container into the receiver container.
// Mounts already present in the receiver (by name) are preserved (receiver takes precedence).
// New mounts from the giver are appended with deep copy to prevent aliasing.
func mergeVolumeMounts(giver corev1.Container, receiver *corev1.Container) {
	for _, mount := range giver.VolumeMounts {
		found := false
		for _, existingMount := range receiver.VolumeMounts {
			if existingMount.Name == mount.Name {
				found = true
				break
			}
		}
		if !found {
			receiver.VolumeMounts = append(receiver.VolumeMounts, *mount.DeepCopy())
		}
	}
}

// findUserContainer finds a container with the given name in the user-defined containers list.
// If found, returns a deep copy of that container to prevent unintended mutations.
// Otherwise, returns an empty container.
func findUserContainer(name string, customContainers []corev1.Container) corev1.Container {
	for i := range customContainers {
		if customContainers[i].Name == name {
			return *customContainers[i].DeepCopy()
		}
	}

	return corev1.Container{Name: name}
}

// ensureEnvVar ensures that an environment variable is present in the list.
// If it exists, it's replaced. If not, it's added.
func ensureEnvVar(envVars []corev1.EnvVar, envVar corev1.EnvVar) []corev1.EnvVar {
	for i := range envVars {
		if envVars[i].Name == envVar.Name {
			envVars[i] = envVar
			return envVars
		}
	}

	return append(envVars, envVar)
}

type volumeAndMount struct {
	Volume      corev1.Volume
	VolumeMount corev1.VolumeMount
}

func getTLSVolumesAndMounts(
	plugins klioconfig.ClusterPlugins,
) []volumeAndMount {
	var volumesAndMounts []volumeAndMount

	keys := make([]string, 0, len(plugins))
	for k := range plugins {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		pc := plugins[key]
		pcName := pc.Name

		serverSecretName := pc.Spec.ServerSecretName
		if serverSecretName != "" {
			volume := corev1.Volume{
				Name: getServerSecretVolumeName(pcName),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: serverSecretName,
					},
				},
			}

			volumeMount := corev1.VolumeMount{
				Name:      getServerSecretVolumeName(pcName),
				MountPath: klioconfig.GetServerSecretVolumeMountPath(pcName),
			}

			volumesAndMounts = append(volumesAndMounts, volumeAndMount{
				Volume:      volume,
				VolumeMount: volumeMount,
			})
		}

		clientSecretName := pc.Spec.ClientSecretName
		if clientSecretName != "" {
			volume := corev1.Volume{
				Name: getClientSecretVolumeName(pcName),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: clientSecretName,
					},
				},
			}

			volumeMount := corev1.VolumeMount{
				Name:      getClientSecretVolumeName(pcName),
				MountPath: klioconfig.GetClientSecretVolumeMountPath(pcName),
			}

			volumesAndMounts = append(volumesAndMounts, volumeAndMount{
				Volume:      volume,
				VolumeMount: volumeMount,
			})
		}
	}

	return volumesAndMounts
}

func getServerSecretVolumeName(hostName string) string {
	return hostName + "-server-certs"
}

func getClientSecretVolumeName(hostName string) string {
	return hostName + "-client-certs"
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

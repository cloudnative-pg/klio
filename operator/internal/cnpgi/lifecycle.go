package cnpgi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

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

const pgdata = "/var/lib/postgresql/data/pgdata"

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
	pluginConfig := cluster.GetRecoverySourcePlugin()

	if pluginConfig == nil || pluginConfig.Name != PluginName {
		contextLogger.Debug("cluster does not use the this plugin for recovery, skipping")
		return nil, nil
	}

	cfg, err := parsePluginConfiguration(pluginConfig)
	if err != nil {
		contextLogger.Error(err, "failed to parse plugin configuration")
		return nil, fmt.Errorf("failed to parse plugin configuration: %w", err)
	}

	if cfg.BackupName == "" {
		contextLogger.Warning("no backup name specified in the configuration, returning error", "cfg", cfg)
		return nil, errors.New("no backupName specified")
	}

	if cfg.ClusterName == "" {
		contextLogger.Debug("no cluster name specified in the configuration, using default cluster name", "clusterName",
			cluster.Name)
		cfg.ClusterName = cluster.Name
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

	var backup cnpgv1.Backup
	if err := impl.Client.Get(
		ctx,
		client.ObjectKey{Name: cfg.BackupName, Namespace: cluster.Namespace},
		&backup,
	); err != nil {
		contextLogger.Error(err, "failed to get backup object")
		return nil, fmt.Errorf("failed to get backup object: %w", err)
	}

	if err := validateBackupForRestore(backup); err != nil {
		contextLogger.Error(err, "while validating backup for restore")
		return nil, err
	}

	mutatedJob := job.DeepCopy()

	if err := reconcilePodSpec(cluster, &mutatedJob.Spec.Template.Spec, jobRole, reconcilePodSpecConfiguration{
		pluginConf: cfg,
		sidecarsToEnrich: []corev1.Container{
			{Name: "klio-plugin", Args: []string{"cnpgi", "restore", backup.Status.BackupID, pgdata}},
		},
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

func validateBackupForRestore(backup cnpgv1.Backup) error {
	if backup.Spec.Method != cnpgv1.BackupMethodPlugin {
		return fmt.Errorf(
			"trying to restore from a backup that is not using the plugin method. '%s' has method: %s",
			backup.Name,
			backup.Spec.Method,
		)
	}

	if backup.Spec.PluginConfiguration == nil {
		return fmt.Errorf(
			"trying to restore from a backup that has no plugin configuration. '%s' has no plugin configuration",
			backup.Name,
		)
	}

	if backup.Spec.PluginConfiguration.Name != PluginName {
		return fmt.Errorf(
			"trying to restore from a backup that is not using the plugin configuration. '%s' has plugin: %v, expected: %s",
			backup.Name,
			backup.Spec.PluginConfiguration.Name,
			PluginName,
		)
	}

	if backup.Status.Phase != cnpgv1.BackupPhaseCompleted {
		return fmt.Errorf(
			"trying to restore form an uncompleted backup. '%s' is not completed, phase: %s",
			backup.Name,
			backup.Status.Phase,
		)
	}

	if backup.Status.BackupID == "" {
		return fmt.Errorf(
			"trying to restore from a backup that has no backup ID. '%s' has no backup ID",
			backup.Name,
		)
	}

	return nil
}

func (impl LifecycleImplementation) reconcilePod(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
	request *lifecycle.OperatorLifecycleRequest,
) (*lifecycle.OperatorLifecycleResponse, error) {
	contextLogger := log.FromContext(ctx).WithName("klio-pod-lifecycle")

	cfg, err := newConfigFromCluster(cluster)
	if errors.Is(err, errPluginNotFound) {
		contextLogger.Debug("Plugin not found in cluster definition, skipping reconciliation")
		return nil, nil
	}
	if errors.Is(err, errPluginNotEnabled) {
		contextLogger.Debug("Plugin found but not enabled in cluster definition, skipping reconciliation")
		return nil, nil
	}
	if err != nil {
		contextLogger.Error(err, "Failed to create config from cluster definition")
		return nil, fmt.Errorf("error creating config from cluster definition: %w", err)
	}

	pod, err := decoder.DecodePodJSON(request.GetObjectDefinition())
	if err != nil {
		return nil, err
	}

	contextLogger = log.FromContext(ctx).WithName("klio-pod-lifecycle").
		WithValues("podName", pod.Name)

	mutatedPod := pod.DeepCopy()

	if err := reconcilePodSpec(
		cluster,
		&mutatedPod.Spec,
		"postgres",
		reconcilePodSpecConfiguration{
			pluginConf: cfg,
			sidecarsToEnrich: []corev1.Container{
				{Name: "klio-wal", Args: []string{"send-wal"}},
				{Name: "klio-plugin", Args: []string{"cnpgi", "instance"}},
			},
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

type reconcilePodSpecConfiguration struct {
	pluginConf       *pluginConfiguration
	sidecarsToEnrich []corev1.Container
}

// reconcilePodSpec reconciles the pod spec to include the klio server sidecar and its configuration.
//
//nolint:cyclop
func reconcilePodSpec(
	cluster *cnpgv1.Cluster,
	spec *corev1.PodSpec,
	mainContainerName string,
	cfg reconcilePodSpecConfiguration,
) error {
	spec.Volumes = ensureVolume(spec.Volumes, corev1.Volume{
		Name: "klio-server-tls",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: cfg.pluginConf.ServerSecretName,
			},
		},
	})

	sidecarTemplate := corev1.Container{
		Image:           "registry.dev:5000/klio-testing:dev",
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
			{Name: "SOURCE_DSN", Value: "user=postgres replication=yes application_name=klio"},
			{Name: "SOURCE_STANDARD_DSN", Value: "user=postgres application_name=klio"},
			{Name: "SOURCE_SLOT", Value: "klio"},
			{Name: "CLIENT_BASE_URL", Value: "https://" + net.JoinHostPort(cfg.pluginConf.ServerAddress, "51515")},
			{Name: "CLIENT_BASE_SERVER_CERT_PATH", Value: "/certs/tls.crt"},
			{Name: "CLIENT_BASE_HOSTNAME", Value: cfg.pluginConf.ClusterName},
			{Name: "CLIENT_BASE_USERNAME", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.pluginConf.ClientSecretName,
					},
					Key: corev1.BasicAuthUsernameKey,
				},
			}},
			{Name: "CLIENT_BASE_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.pluginConf.ClientSecretName,
					},
					Key: corev1.BasicAuthPasswordKey,
				},
			}},
			{Name: "CLIENT_WAL_ADDRESS", Value: net.JoinHostPort(cfg.pluginConf.ServerAddress, "52000")},
			{Name: "CLIENT_WAL_CLUSTER_NAME", Value: cfg.pluginConf.ClusterName},
			{Name: "CLIENT_WAL_SERVER_CERT_PATH", Value: "/certs/tls.crt"},
			{Name: "CLIENT_WAL_USERNAME", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.pluginConf.ClientSecretName,
					},
					Key: corev1.BasicAuthUsernameKey,
				},
			}},
			{Name: "CLIENT_WAL_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.pluginConf.ClientSecretName,
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
	for _, env := range mainContainer.Env {
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

	for _, baseSidecar := range cfg.sidecarsToEnrich {
		sidecar := sidecarTemplate.DeepCopy()
		sidecar.Name = baseSidecar.Name
		sidecar.Args = baseSidecar.Args
		if cfg.pluginConf.EnablePPROF {
			sidecar.Args = append(sidecar.Args, "--pprof-server=0:6060")
		}
		if err := injectPluginSidecarPodSpec(spec, sidecar, mainContainerName); err != nil {
			return fmt.Errorf("failed to inject sidecar %s: %w", sidecar.Name, err)
		}
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

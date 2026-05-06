package cnpgi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/lifecycle"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

const (
	testPodName          = "test-pod"
	testClusterName      = "test-cluster"
	testClusterNamespace = "test-namespace"

	argPodName          = "--pod-name"
	argClusterName      = "--cluster-name"
	argClusterNamespace = "--cluster-namespace"
	argConfig           = "--config"

	expectedArchiveConfigPath = "/var/lib/postgresql/klio/klio-archive"
)

func TestCnpgGroupVersion(t *testing.T) {
	defaultGroup := cnpgv1.SchemeGroupVersion.Group
	defaultVersion := cnpgv1.SchemeGroupVersion.Version

	tests := []struct {
		name            string
		cluster         *cnpgv1.Cluster
		expectedGroup   string
		expectedVersion string
	}{
		{
			name: "both set in TypeMeta",
			cluster: &cnpgv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "custom.example.io/v2",
					Kind:       "Cluster",
				},
			},
			expectedGroup:   "custom.example.io",
			expectedVersion: "v2",
		},
		{
			name:            "both empty falls back to defaults",
			cluster:         &cnpgv1.Cluster{},
			expectedGroup:   defaultGroup,
			expectedVersion: defaultVersion,
		},
		{
			name: "only group empty falls back to default group",
			cluster: &cnpgv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v3",
					Kind:       "Cluster",
				},
			},
			expectedGroup:   defaultGroup,
			expectedVersion: "v3",
		},
		{
			name: "only version empty falls back to default version",
			cluster: &cnpgv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "custom.example.io/",
					Kind:       "Cluster",
				},
			},
			expectedGroup:   "custom.example.io",
			expectedVersion: defaultVersion,
		},
		{
			name: "standard CNPG APIVersion",
			cluster: &cnpgv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: cnpgv1.SchemeGroupVersion.String(),
					Kind:       "Cluster",
				},
			},
			expectedGroup:   defaultGroup,
			expectedVersion: defaultVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, version := cnpgGroupVersion(tt.cluster)
			assert.Equal(t, tt.expectedGroup, group)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

func TestFindUserContainer(t *testing.T) {
	tests := []struct {
		name              string
		containerName     string
		customContainers  []corev1.Container
		expectedContainer corev1.Container
	}{
		{
			name:          "container found",
			containerName: KlioPluginContainerName,
			customContainers: []corev1.Container{
				{
					Name:  KlioPluginContainerName,
					Image: "custom-image:latest",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
			expectedContainer: corev1.Container{
				Name:  KlioPluginContainerName,
				Image: "custom-image:latest",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		},
		{
			name:          "container not found",
			containerName: KlioPluginContainerName,
			customContainers: []corev1.Container{
				{
					Name:  "klio-wal",
					Image: "other-image:latest",
				},
			},
			expectedContainer: corev1.Container{Name: KlioPluginContainerName},
		},
		{
			name:              "empty list",
			containerName:     KlioPluginContainerName,
			customContainers:  []corev1.Container{},
			expectedContainer: corev1.Container{Name: KlioPluginContainerName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUserContainer(tt.containerName, tt.customContainers)
			assert.Equal(t, tt.expectedContainer, result)
		})
	}
}

func TestEnsureEnvVar(t *testing.T) {
	tests := []struct {
		name        string
		envVars     []corev1.EnvVar
		newEnvVar   corev1.EnvVar
		expectedEnv []corev1.EnvVar
	}{
		{
			name: "add new env var",
			envVars: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "existing"},
			},
			newEnvVar: corev1.EnvVar{Name: "NEW_VAR", Value: "new"},
			expectedEnv: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "existing"},
				{Name: "NEW_VAR", Value: "new"},
			},
		},
		{
			name: "replace existing env var",
			envVars: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "old"},
			},
			newEnvVar: corev1.EnvVar{Name: "EXISTING_VAR", Value: "new"},
			expectedEnv: []corev1.EnvVar{
				{Name: "EXISTING_VAR", Value: "new"},
			},
		},
		{
			name:      "add to empty list",
			envVars:   []corev1.EnvVar{},
			newEnvVar: corev1.EnvVar{Name: "NEW_VAR", Value: "new"},
			expectedEnv: []corev1.EnvVar{
				{Name: "NEW_VAR", Value: "new"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureEnvVar(tt.envVars, tt.newEnvVar)
			assert.Equal(t, tt.expectedEnv, result)
		})
	}
}

//nolint:dupl // Similar test structure for different merge functions is acceptable
func TestMergeEnvironmentVariables(t *testing.T) {
	t.Run("merge without duplicates", func(t *testing.T) {
		giver := corev1.Container{
			Env: []corev1.EnvVar{
				{Name: "VAR1", Value: "value1"},
				{Name: "VAR2", Value: "value2"},
			},
		}
		receiver := corev1.Container{
			Env: []corev1.EnvVar{{Name: "VAR3", Value: "value3"}},
		}
		mergeEnvironmentVariables(giver, &receiver)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "VAR3", Value: "value3"},
			{Name: "VAR1", Value: "value1"},
			{Name: "VAR2", Value: "value2"},
		}, receiver.Env)
	})

	t.Run("skip duplicates", func(t *testing.T) {
		giver := corev1.Container{
			Env: []corev1.EnvVar{
				{Name: "VAR1", Value: "giver-value"},
				{Name: "VAR2", Value: "value2"},
			},
		}
		receiver := corev1.Container{
			Env: []corev1.EnvVar{{Name: "VAR1", Value: "receiver-value"}},
		}
		mergeEnvironmentVariables(giver, &receiver)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "VAR1", Value: "receiver-value"},
			{Name: "VAR2", Value: "value2"},
		}, receiver.Env)
	})

	t.Run("merge into empty receiver", func(t *testing.T) {
		giver := corev1.Container{Env: []corev1.EnvVar{{Name: "VAR1", Value: "value1"}}}
		receiver := corev1.Container{Env: []corev1.EnvVar{}}
		mergeEnvironmentVariables(giver, &receiver)
		assert.Equal(t, []corev1.EnvVar{{Name: "VAR1", Value: "value1"}}, receiver.Env)
	})

	t.Run("merge from empty giver", func(t *testing.T) {
		giver := corev1.Container{Env: []corev1.EnvVar{}}
		receiver := corev1.Container{
			Env: []corev1.EnvVar{{Name: "VAR1", Value: "receiver-value"}},
		}
		mergeEnvironmentVariables(giver, &receiver)
		assert.Equal(t, []corev1.EnvVar{
			{Name: "VAR1", Value: "receiver-value"},
		}, receiver.Env)
	})
}

//nolint:dupl // Similar test structure for different merge functions is acceptable
func TestMergeVolumeMounts(t *testing.T) {
	t.Run("merge without duplicates", func(t *testing.T) {
		giver := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{
				{Name: "mount1", MountPath: "/path1"},
				{Name: "mount2", MountPath: "/path2"},
			},
		}
		receiver := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{{Name: "mount3", MountPath: "/path3"}},
		}
		mergeVolumeMounts(giver, &receiver)
		assert.Equal(t, []corev1.VolumeMount{
			{Name: "mount3", MountPath: "/path3"},
			{Name: "mount1", MountPath: "/path1"},
			{Name: "mount2", MountPath: "/path2"},
		}, receiver.VolumeMounts)
	})

	t.Run("skip duplicates by name", func(t *testing.T) {
		giver := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{
				{Name: "mount1", MountPath: "/giver/path"},
				{Name: "mount2", MountPath: "/path2"},
			},
		}
		receiver := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{{Name: "mount1", MountPath: "/receiver/path"}},
		}
		mergeVolumeMounts(giver, &receiver)
		assert.Equal(t, []corev1.VolumeMount{
			{Name: "mount1", MountPath: "/receiver/path"},
			{Name: "mount2", MountPath: "/path2"},
		}, receiver.VolumeMounts)
	})

	t.Run("merge into empty receiver", func(t *testing.T) {
		giver := corev1.Container{VolumeMounts: []corev1.VolumeMount{{Name: "mount1", MountPath: "/path1"}}}
		receiver := corev1.Container{VolumeMounts: []corev1.VolumeMount{}}
		mergeVolumeMounts(giver, &receiver)
		assert.Equal(t, []corev1.VolumeMount{{Name: "mount1", MountPath: "/path1"}}, receiver.VolumeMounts)
	})

	t.Run("merge from empty giver", func(t *testing.T) {
		giver := corev1.Container{VolumeMounts: []corev1.VolumeMount{}}
		receiver := corev1.Container{
			VolumeMounts: []corev1.VolumeMount{{Name: "mount1", MountPath: "/receiver/path"}},
		}
		mergeVolumeMounts(giver, &receiver)
		assert.Equal(t, []corev1.VolumeMount{
			{Name: "mount1", MountPath: "/receiver/path"},
		}, receiver.VolumeMounts)
	})
}

func TestBuildInstanceSidecarTemplate(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Name = testPodName

	cluster := &cnpgv1.Cluster{}
	cluster.Name = testClusterName
	cluster.Namespace = testClusterNamespace

	t.Run("with user customizations", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Containers: []corev1.Container{
					{
						Name:  KlioPluginContainerName,
						Image: "custom-image:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
						// Add custom env and mounts to check preservation
						Env: []corev1.EnvVar{
							{Name: "CUSTOM_VAR", Value: "custom-value"},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "custom-mount", MountPath: "/custom/path"},
						},
					},
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC, klioconfig.ArchiveConfigKey)

		// Klio required values are set
		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, expectedArchiveConfigPath,
		}, result.Args)
		assertContainerNameEnvVar(t, result)

		// User customizations are preserved
		assert.Equal(t, "custom-image:latest", result.Image)
		assert.Equal(t, resource.MustParse("512Mi"), result.Resources.Limits[corev1.ResourceMemory])
		assert.Len(t, result.VolumeMounts, 1, "Custom volume mount should be preserved")
		assert.Equal(t, "custom-mount", result.VolumeMounts[0].Name)

		// Check custom env is preserved
		customVarFound := false
		for _, env := range result.Env {
			if env.Name == "CUSTOM_VAR" {
				assert.Equal(t, "custom-value", env.Value)
				customVarFound = true
			}
		}
		assert.True(t, customVarFound, "CUSTOM_VAR was not preserved")
	})

	t.Run("with tier2 backup enabled", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Tier2: &kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup:   true,
					EnableRecovery: true,
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC, "klio-archive")

		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, expectedArchiveConfigPath,
		}, result.Args)
		assert.NotContains(t, result.Args, "--tier1")
		assert.NotContains(t, result.Args, "--enable-tier2-backup")
		assert.NotContains(t, result.Args, "--enable-tier2-recovery")
		assertContainerNameEnvVar(t, result)
	})

	t.Run("with nil tier2", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC, "klio-archive")

		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, expectedArchiveConfigPath,
		}, result.Args)
		assert.NotContains(t, result.Args, "--tier1")
		assert.NotContains(t, result.Args, "--enable-tier2-backup")
		assert.NotContains(t, result.Args, "--enable-tier2-recovery")
		assertContainerNameEnvVar(t, result)
	})

	t.Run("with nil clusterPC", func(t *testing.T) {
		result := buildInstanceSidecarTemplate(pod, cluster, nil, klioconfig.ArchiveConfigKey)

		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, expectedArchiveConfigPath,
		}, result.Args)
	})

	t.Run("with custom config key", func(t *testing.T) {
		result := buildInstanceSidecarTemplate(pod, cluster, nil, "source-server")

		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, "/var/lib/postgresql/klio/source-server",
		}, result.Args)
	})

	t.Run("with user customizations for different container", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Containers: []corev1.Container{
					{
						Name:  "other-container", // This should be ignored
						Image: "custom-image:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC, klioconfig.ArchiveConfigKey)

		// Should get the default template, not the custom one
		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			argPodName, testPodName,
			argClusterName, testClusterName,
			argClusterNamespace, testClusterNamespace,
			argConfig, expectedArchiveConfigPath,
		}, result.Args)
		assert.Empty(t, result.Image, "Image should not be set from non-matching container")
		assert.Nil(t, result.Resources.Limits, "Resources should not be set from non-matching container")
		assertContainerNameEnvVar(t, result)
	})
}

// assertContainerNameEnvVar is a helper to check that CONTAINER_NAME env var is set to "klio-plugin".
func assertContainerNameEnvVar(t *testing.T, container corev1.Container) {
	t.Helper()
	for _, env := range container.Env {
		if env.Name == "CONTAINER_NAME" {
			assert.Equal(t, KlioPluginContainerName, env.Value)
			return
		}
	}
	t.Errorf("CONTAINER_NAME env var not found")
}

func TestUserContainerCustomizationsPreserved(t *testing.T) {
	t.Run("user container with all customizations", func(t *testing.T) {
		customContainers := []corev1.Container{
			{
				Name:  KlioPluginContainerName,
				Image: "custom-image:v1.0",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1Gi"),
						corev1.ResourceCPU:    resource.MustParse("500m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
						corev1.ResourceCPU:    resource.MustParse("250m"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:  new(int64(1000)),
					RunAsGroup: new(int64(1000)),
				},
				Env: []corev1.EnvVar{
					{Name: "CUSTOM_VAR", Value: "custom-value"},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "custom-mount", MountPath: "/custom/path"},
				},
				Ports: []corev1.ContainerPort{
					{Name: "metrics", ContainerPort: 9090},
				},
			},
		}

		result := findUserContainer(KlioPluginContainerName, customContainers)

		// Verify all customizations are preserved
		assert.Equal(t, KlioPluginContainerName, result.Name)
		assert.Equal(t, "custom-image:v1.0", result.Image)
		assert.Equal(t, resource.MustParse("1Gi"), result.Resources.Limits[corev1.ResourceMemory])
		assert.Equal(t, resource.MustParse("500m"), result.Resources.Limits[corev1.ResourceCPU])
		assert.Equal(t, resource.MustParse("512Mi"), result.Resources.Requests[corev1.ResourceMemory])
		assert.Equal(t, resource.MustParse("250m"), result.Resources.Requests[corev1.ResourceCPU])
		assert.NotNil(t, result.SecurityContext)
		assert.Equal(t, int64(1000), *result.SecurityContext.RunAsUser)
		assert.Equal(t, int64(1000), *result.SecurityContext.RunAsGroup)
		assert.Len(t, result.Env, 1)
		assert.Equal(t, "CUSTOM_VAR", result.Env[0].Name)
		assert.Equal(t, "custom-value", result.Env[0].Value)
		assert.Len(t, result.VolumeMounts, 1)
		assert.Equal(t, "custom-mount", result.VolumeMounts[0].Name)
		assert.Equal(t, "/custom/path", result.VolumeMounts[0].MountPath)
		assert.Len(t, result.Ports, 1)
		assert.Equal(t, "metrics", result.Ports[0].Name)
		assert.Equal(t, int32(9090), result.Ports[0].ContainerPort)
	})
}

// applyPatch applies a JSON patch from a lifecycle response to the original
// object and returns the patched result.
//
//nolint:ireturn
func applyPatch[T client.Object](t *testing.T, resp *lifecycle.OperatorLifecycleResponse, original T) T {
	t.Helper()

	patch, err := jsonpatch.DecodePatch(resp.GetJsonPatch())
	require.NoError(t, err, "failed to decode JSON patch")

	originalJSON, err := json.Marshal(original)
	require.NoError(t, err)

	patchedJSON, err := patch.Apply(originalJSON)
	require.NoError(t, err, "failed to apply JSON patch")

	result, ok := original.DeepCopyObject().(T)
	require.True(t, ok, "failed to cast patched object to original type")
	require.NoError(t, json.Unmarshal(patchedJSON, result))

	return result
}

// findInitContainer returns the init container with the given name, or nil.
func findInitContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}

	return nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, kliov1alpha1.AddToScheme(s))
	require.NoError(t, cnpgv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))

	return s
}

func buildLifecycleRequest(
	t *testing.T,
	cluster *cnpgv1.Cluster,
	obj metav1.Object,
) *lifecycle.OperatorLifecycleRequest {
	t.Helper()
	clusterJSON, err := json.Marshal(cluster)
	require.NoError(t, err)
	objJSON, err := json.Marshal(obj)
	require.NoError(t, err)

	return &lifecycle.OperatorLifecycleRequest{
		OperationType: &lifecycle.OperatorOperationType{
			Type: lifecycle.OperatorOperationType_TYPE_CREATE,
		},
		ClusterDefinition: clusterJSON,
		ObjectDefinition:  objJSON,
	}
}

// clusterWithKlioPlugin returns a cluster with the Klio archive plugin
// referencing a PluginConfiguration named testPluginConfigName.
func clusterWithKlioPlugin() *cnpgv1.Cluster {
	const testPluginConfigName = "missing-plugin-config"
	return &cnpgv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cnpgv1.SchemeGroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testClusterNamespace,
		},
		Spec: cnpgv1.ClusterSpec{
			Plugins: []cnpgv1.PluginConfiguration{
				{
					Name:    klioconfig.PluginName,
					Enabled: new(true),
					Parameters: map[string]string{
						klioconfig.PluginConfigurationRefParam: testPluginConfigName,
					},
				},
			},
		},
	}
}

func TestReconcilePodPluginSelection(t *testing.T) {
	scheme := newTestScheme(t)

	const (
		sourcePCName   = "source-pc"
		sourceCluster  = "source-cluster"
		targetPodName  = "replica-cluster-1"
		replicaCluster = "replica-cluster"
	)

	makeReplicaCluster := func(withArchive bool) *cnpgv1.Cluster {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: cnpgv1.SchemeGroupVersion.String(),
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      replicaCluster,
				Namespace: testClusterNamespace,
			},
			Spec: cnpgv1.ClusterSpec{
				ReplicaCluster: &cnpgv1.ReplicaClusterConfiguration{
					Enabled: new(true),
					Source:  sourceCluster,
				},
				ExternalClusters: []cnpgv1.ExternalCluster{
					{
						Name: sourceCluster,
						PluginConfiguration: &cnpgv1.PluginConfiguration{
							Name:    klioconfig.PluginName,
							Enabled: new(true),
							Parameters: map[string]string{
								klioconfig.PluginConfigurationRefParam: sourcePCName,
							},
						},
					},
				},
			},
			Status: cnpgv1.ClusterStatus{
				TargetPrimary: targetPodName,
			},
		}

		if withArchive {
			cluster.Spec.Plugins = []cnpgv1.PluginConfiguration{
				{
					Name:    klioconfig.PluginName,
					Enabled: new(true),
					Parameters: map[string]string{
						klioconfig.PluginConfigurationRefParam: "archive-pc",
					},
				},
			}
		}

		return cluster
	}

	sourcePC := &kliov1alpha1.PluginConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourcePCName,
			Namespace: testClusterNamespace,
		},
		Spec: kliov1alpha1.PluginConfigurationSpec{
			ClusterName:      sourceCluster,
			ServerAddress:    "klio-server.example.com",
			ClientSecretName: "client-secret",
			ServerSecretName: "server-secret",
			Containers: []corev1.Container{
				{
					Name:  KlioPluginContainerName,
					Image: "source-image:latest",
				},
			},
		},
	}

	makePod := func(name string) *corev1.Pod {
		return &corev1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testClusterNamespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "postgres"},
				},
			},
		}
	}

	t.Run("no archive plugin and not a replica cluster returns nil", func(t *testing.T) {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: cnpgv1.SchemeGroupVersion.String(),
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      testClusterName,
				Namespace: testClusterNamespace,
			},
			Spec: cnpgv1.ClusterSpec{},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, makePod("pod-1"))

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		assert.Nil(t, resp)
	})

	t.Run("no archive plugin, replica cluster, non-designed-primary pod returns nil", func(t *testing.T) {
		cluster := makeReplicaCluster(false)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sourcePC.DeepCopy()).
			Build()
		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, makePod("replica-cluster-2"))

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		assert.Nil(t, resp, "non-primary pod should not get a sidecar")
	})

	t.Run("no archive plugin, replica cluster primary uses source plugin configuration", func(t *testing.T) {
		cluster := makeReplicaCluster(false)
		pod := makePod(targetPodName)
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sourcePC.DeepCopy()).
			Build()
		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, pod)

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		require.NotNil(t, resp, "primary pod of replica cluster should get a sidecar")

		patchedPod := applyPatch(t, resp, pod)
		sidecar := findInitContainer(patchedPod, KlioPluginContainerName)
		require.NotNil(t, sidecar, "sidecar init container should be present")
		assert.Equal(t, "source-image:latest", sidecar.Image,
			"sidecar should use the image from the source PluginConfiguration")
		assert.Contains(t, sidecar.Args, "--config",
			"sidecar args should contain --config")
		assert.Contains(t, sidecar.Args, "/var/lib/postgresql/klio/"+sourceCluster,
			"sidecar config path should reference the source cluster key")
	})

	t.Run("no archive plugin, replica cluster primary, source plugin missing returns nil", func(t *testing.T) {
		cluster := makeReplicaCluster(false)
		// No external cluster klio plugin configured.
		cluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{
			{Name: sourceCluster},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, makePod(targetPodName))

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		assert.Nil(t, resp, "should return nil when replica source has no klio plugin")
	})

	t.Run("archive plugin present takes precedence over replica source", func(t *testing.T) {
		cluster := makeReplicaCluster(true)
		pod := makePod(targetPodName)
		archivePC := &kliov1alpha1.PluginConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "archive-pc",
				Namespace: testClusterNamespace,
			},
			Spec: kliov1alpha1.PluginConfigurationSpec{
				ClusterName:      replicaCluster,
				ServerAddress:    "klio-server.example.com",
				ClientSecretName: "client-secret",
				ServerSecretName: "server-secret",
				Containers: []corev1.Container{
					{
						Name:  KlioPluginContainerName,
						Image: "archive-image:latest",
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(archivePC.DeepCopy(), sourcePC.DeepCopy()).
			Build()
		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, pod)

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		require.NotNil(t, resp)

		patchedPod := applyPatch(t, resp, pod)
		sidecar := findInitContainer(patchedPod, KlioPluginContainerName)
		require.NotNil(t, sidecar, "sidecar init container should be present")
		assert.Equal(t, "archive-image:latest", sidecar.Image,
			"sidecar should use the image from the archive PluginConfiguration, not the source")
		assert.Contains(t, sidecar.Args, "/var/lib/postgresql/klio/"+klioconfig.ArchiveConfigKey,
			"sidecar config path should reference the archive key")
	})
}

func TestReconcilePodRequeue(t *testing.T) {
	scheme := newTestScheme(t)

	t.Run("returns error when PluginConfiguration does not exist", func(t *testing.T) {
		cluster := clusterWithKlioPlugin()
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		impl := LifecycleImplementation{Client: fakeClient}
		pod := &corev1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster-1",
				Namespace: testClusterNamespace,
			},
		}
		req := buildLifecycleRequest(t, cluster, pod)

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.Error(t, err, "missing PluginConfiguration must return an error")
		assert.Nil(t, resp)
		assert.True(t, klioconfig.IsPluginConfigurationNotFound(err))
	})

	t.Run("propagates non-NotFound errors instead of requeuing", func(t *testing.T) {
		cluster := clusterWithKlioPlugin()
		injectedErr := errors.New("simulated API server unavailable")
		fakeClient := interceptor.NewClient(
			fake.NewClientBuilder().WithScheme(scheme).Build(),
			interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch,
					key client.ObjectKey, obj client.Object,
					opts ...client.GetOption,
				) error {
					if _, ok := obj.(*kliov1alpha1.PluginConfiguration); ok {
						return injectedErr
					}

					return c.Get(ctx, key, obj, opts...)
				},
			},
		)

		impl := LifecycleImplementation{Client: fakeClient}
		pod := &corev1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-cluster-1",
				Namespace: testClusterNamespace,
			},
		}
		req := buildLifecycleRequest(t, cluster, pod)

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.Error(t, err, "non-NotFound errors must propagate")
		assert.Nil(t, resp)
		assert.ErrorContains(t, err, "simulated API server unavailable")
	})

	t.Run("cluster without Klio plugin returns nil (no requeue)", func(t *testing.T) {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: cnpgv1.SchemeGroupVersion.String(),
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-plugin-cluster",
				Namespace: testClusterNamespace,
			},
			Spec: cnpgv1.ClusterSpec{},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		impl := LifecycleImplementation{Client: fakeClient}
		pod := &corev1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-plugin-cluster-1",
				Namespace: testClusterNamespace,
			},
		}
		req := buildLifecycleRequest(t, cluster, pod)

		resp, err := impl.reconcilePod(context.Background(), cluster, req)

		require.NoError(t, err)
		assert.Nil(t, resp, "cluster without Klio plugin should return nil response")
	})
}

func TestReconcileJobRequeue(t *testing.T) {
	scheme := newTestScheme(t)

	t.Run("returns error when archive PluginConfiguration is missing", func(t *testing.T) {
		cluster := clusterWithKlioPlugin()
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "dummy", Namespace: testClusterNamespace},
		})

		resp, err := impl.reconcileJob(context.Background(), cluster, req)

		require.Error(t, err, "missing PluginConfiguration must return an error")
		assert.Nil(t, resp)
		assert.True(t, klioconfig.IsPluginConfigurationNotFound(err))
	})

	t.Run("propagates non-NotFound errors instead of requeuing", func(t *testing.T) {
		cluster := clusterWithKlioPlugin()
		injectedErr := errors.New("simulated etcd timeout")
		fakeClient := interceptor.NewClient(
			fake.NewClientBuilder().WithScheme(scheme).Build(),
			interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch,
					key client.ObjectKey, obj client.Object,
					opts ...client.GetOption,
				) error {
					if _, ok := obj.(*kliov1alpha1.PluginConfiguration); ok {
						return injectedErr
					}

					return c.Get(ctx, key, obj, opts...)
				},
			},
		)

		impl := LifecycleImplementation{Client: fakeClient}
		req := buildLifecycleRequest(t, cluster, &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "dummy", Namespace: testClusterNamespace},
		})

		resp, err := impl.reconcileJob(context.Background(), cluster, req)

		require.Error(t, err, "non-NotFound errors must propagate")
		assert.Nil(t, resp)
		assert.ErrorContains(t, err, "simulated etcd timeout")
	})
}

func TestKlioRequiredValuesOverride(t *testing.T) {
	t.Run("klio required values override user values", func(t *testing.T) {
		// Start with user container that has conflicting values
		userContainer := corev1.Container{
			Name: KlioPluginContainerName,
			Args: []string{"wrong", "args"},
			Env: []corev1.EnvVar{
				{Name: "CUSTOM_VAR", Value: "custom"},
				{Name: "CONTAINER_NAME", Value: "wrong-value"},
			},
		}

		// Simulate what buildInstanceSidecarTemplate does
		sidecar := userContainer
		sidecar.Name = KlioPluginContainerName       // Klio overrides name
		sidecar.Args = []string{"cnpgi", "instance"} // Klio overrides args
		sidecar.Env = ensureEnvVar(sidecar.Env, corev1.EnvVar{
			Name:  "CONTAINER_NAME",
			Value: KlioPluginContainerName,
		}) // Klio ensures its env var

		// Verify Klio required values are set
		assert.Equal(t, KlioPluginContainerName, sidecar.Name)
		assert.Equal(t, []string{"cnpgi", "instance"}, sidecar.Args)

		// Verify CONTAINER_NAME was overridden
		containerNameFound := false
		customVarFound := false
		for _, env := range sidecar.Env {
			if env.Name == "CONTAINER_NAME" {
				assert.Equal(t, KlioPluginContainerName, env.Value)
				containerNameFound = true
			}
			if env.Name == "CUSTOM_VAR" {
				assert.Equal(t, "custom", env.Value)
				customVarFound = true
			}
		}
		assert.True(t, containerNameFound, "CONTAINER_NAME should be set")
		assert.True(t, customVarFound, "CUSTOM_VAR should be preserved")
	})
}

func TestSidecarSecurityContext(t *testing.T) {
	t.Run("returns UID/GID 26 on vanilla Kubernetes", func(t *testing.T) {
		sc := sidecarSecurityContext(false)
		require.NotNil(t, sc)
		assert.Equal(t, int64(26), *sc.RunAsUser)
		assert.Equal(t, int64(26), *sc.RunAsGroup)
	})

	t.Run("returns nil on OpenShift", func(t *testing.T) {
		sc := sidecarSecurityContext(true)
		assert.Nil(t, sc)
	})
}

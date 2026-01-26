package cnpgi

import (
	"testing"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

func TestFindUserContainer(t *testing.T) {
	tests := []struct {
		name              string
		containerName     string
		customContainers  []corev1.Container
		expectedContainer corev1.Container
	}{
		{
			name:          "container found",
			containerName: "klio-plugin",
			customContainers: []corev1.Container{
				{
					Name:  "klio-plugin",
					Image: "custom-image:latest",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
				},
			},
			expectedContainer: corev1.Container{
				Name:  "klio-plugin",
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
			containerName: "klio-plugin",
			customContainers: []corev1.Container{
				{
					Name:  "klio-wal",
					Image: "other-image:latest",
				},
			},
			expectedContainer: corev1.Container{Name: "klio-plugin"},
		},
		{
			name:              "empty list",
			containerName:     "klio-plugin",
			customContainers:  []corev1.Container{},
			expectedContainer: corev1.Container{Name: "klio-plugin"},
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
	pod.Name = "test-pod"

	cluster := &cnpgv1.Cluster{}
	cluster.Name = "test-cluster"
	cluster.Namespace = "test-namespace"

	t.Run("with user customizations", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Containers: []corev1.Container{
					{
						Name:  "klio-plugin",
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

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC)

		// Klio required values are set
		assert.Equal(t, "klio-plugin", result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			"--pod-name", "test-pod",
			"--cluster-name", "test-cluster",
			"--cluster-namespace", "test-namespace",
			"--config", "/var/lib/postgresql/klio/klio-archive",
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
				Tier2: kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup: true,
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC)

		assert.Equal(t, "klio-plugin", result.Name)
		assert.Contains(t, result.Args, "--enable-tier2-backup")
		assert.NotContains(t, result.Args, "--enable-tier2-recovery")
	})

	t.Run("with tier2 recovery enabled", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Tier2: kliov1alpha1.Tier2PluginConfiguration{
					EnableRecovery: true,
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC)

		assert.Equal(t, "klio-plugin", result.Name)
		assert.NotContains(t, result.Args, "--enable-tier2-backup")
		assert.Contains(t, result.Args, "--enable-tier2-recovery")
	})

	t.Run("with both tier2 backup and recovery enabled", func(t *testing.T) {
		clusterPC := &kliov1alpha1.PluginConfiguration{
			Spec: kliov1alpha1.PluginConfigurationSpec{
				Tier2: kliov1alpha1.Tier2PluginConfiguration{
					EnableBackup:   true,
					EnableRecovery: true,
				},
			},
		}

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC)

		assert.Equal(t, "klio-plugin", result.Name)
		assert.Contains(t, result.Args, "--enable-tier2-backup")
		assert.Contains(t, result.Args, "--enable-tier2-recovery")
	})

	t.Run("with nil clusterPC", func(t *testing.T) {
		result := buildInstanceSidecarTemplate(pod, cluster, nil)

		assert.Equal(t, "klio-plugin", result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			"--pod-name", "test-pod",
			"--cluster-name", "test-cluster",
			"--cluster-namespace", "test-namespace",
			"--config", "/var/lib/postgresql/klio/klio-archive",
		}, result.Args)
		assert.NotContains(t, result.Args, "--enable-tier2-backup")
		assert.NotContains(t, result.Args, "--enable-tier2-recovery")
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

		result := buildInstanceSidecarTemplate(pod, cluster, clusterPC)

		// Should get the default template, not the custom one
		assert.Equal(t, "klio-plugin", result.Name)
		assert.Equal(t, []string{
			"cnpgi",
			"instance",
			"--pod-name", "test-pod",
			"--cluster-name", "test-cluster",
			"--cluster-namespace", "test-namespace",
			"--config", "/var/lib/postgresql/klio/klio-archive",
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
			assert.Equal(t, "klio-plugin", env.Value)
			return
		}
	}
	t.Errorf("CONTAINER_NAME env var not found")
}

func TestUserContainerCustomizationsPreserved(t *testing.T) {
	t.Run("user container with all customizations", func(t *testing.T) {
		customContainers := []corev1.Container{
			{
				Name:  "klio-plugin",
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
					RunAsUser:  ptr.To(int64(1000)),
					RunAsGroup: ptr.To(int64(1000)),
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

		result := findUserContainer("klio-plugin", customContainers)

		// Verify all customizations are preserved
		assert.Equal(t, "klio-plugin", result.Name)
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

func TestKlioRequiredValuesOverride(t *testing.T) {
	t.Run("klio required values override user values", func(t *testing.T) {
		// Start with user container that has conflicting values
		userContainer := corev1.Container{
			Name: "klio-plugin",
			Args: []string{"wrong", "args"},
			Env: []corev1.EnvVar{
				{Name: "CUSTOM_VAR", Value: "custom"},
				{Name: "CONTAINER_NAME", Value: "wrong-value"},
			},
		}

		// Simulate what buildInstanceSidecarTemplate does
		sidecar := userContainer
		sidecar.Name = "klio-plugin"                 // Klio overrides name
		sidecar.Args = []string{"cnpgi", "instance"} // Klio overrides args
		sidecar.Env = ensureEnvVar(sidecar.Env, corev1.EnvVar{
			Name:  "CONTAINER_NAME",
			Value: "klio-plugin",
		}) // Klio ensures its env var

		// Verify Klio required values are set
		assert.Equal(t, "klio-plugin", sidecar.Name)
		assert.Equal(t, []string{"cnpgi", "instance"}, sidecar.Args)

		// Verify CONTAINER_NAME was overridden
		containerNameFound := false
		customVarFound := false
		for _, env := range sidecar.Env {
			if env.Name == "CONTAINER_NAME" {
				assert.Equal(t, "klio-plugin", env.Value)
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

package podtemplate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeDoNotDeleteContainers(t *testing.T) {
	base := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "base",
					Image: "test",
				},
				{
					Name:  "wal",
					Image: "toast",
				},
			},
		},
	}

	overlay := corev1.PodTemplateSpec{}

	result, err := Merge(&base, &overlay)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Spec.Containers, 2)
}

func TestOverrideImageMergeVolume(t *testing.T) {
	base := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "base",
					Image: "test",
				},
			},
			Volumes: []corev1.Volume{
				{
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
					Name: "empty",
				},
			},
		},
	}

	overlay := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "base",
					Image: "test_changed",
				},
			},
			Volumes: []corev1.Volume{
				{
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
					Name: "tempdir",
				},
			},
		},
	}

	result, err := Merge(&base, &overlay)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Spec.Volumes, 2)
	assert.Equal(t, result.Spec.Volumes[0].Name, overlay.Spec.Volumes[0].Name)
	assert.Equal(t, result.Spec.Volumes[1].Name, base.Spec.Volumes[0].Name)
	assert.Equal(t, result.Spec.Containers[0].Image, overlay.Spec.Containers[0].Image)
}

func TestMergeEnv(t *testing.T) {
	base := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "base",
					Image: "test",
					Env: []corev1.EnvVar{
						{
							Name:  "test",
							Value: "test",
						},
						{
							Name:  "toast",
							Value: "toast",
						},
					},
				},
			},
		},
	}

	overlay := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "base",
					Env: []corev1.EnvVar{
						{
							Name:  "toast",
							Value: "toast_changed",
						},
						{
							Name:  "sandwich",
							Value: "sandwich",
						},
					},
				},
			},
		},
	}

	result, err := Merge(&base, &overlay)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Spec.Containers, 1)
	assert.Len(t, result.Spec.Containers[0].Env, 3)
	assert.Equal(
		t, corev1.EnvVar{
			Name:  "test",
			Value: "test",
		},
		result.Spec.Containers[0].Env[0],
	)
	assert.Equal(
		t,
		corev1.EnvVar{
			Name:  "toast",
			Value: "toast_changed",
		},
		result.Spec.Containers[0].Env[1],
	)
	assert.Equal(
		t,
		corev1.EnvVar{
			Name:  "sandwich",
			Value: "sandwich",
		},
		result.Spec.Containers[0].Env[2],
	)
}

func TestMergeLabels(t *testing.T) {
	base := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"base-label": "base-value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test", Image: "test"}},
		},
	}

	overlay := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"overlay-label": "overlay-value",
			},
		},
	}

	result, err := Merge(&base, &overlay)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "base-value", result.Labels["base-label"])
	assert.Equal(t, "overlay-value", result.Labels["overlay-label"])
}

func TestMergeAnnotations(t *testing.T) {
	base := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"base-annotation": "base-value",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "test", Image: "test"}},
		},
	}

	overlay := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"overlay-annotation": "overlay-value",
			},
		},
	}

	result, err := Merge(&base, &overlay)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "base-value", result.Annotations["base-annotation"])
	assert.Equal(t, "overlay-value", result.Annotations["overlay-annotation"])
}

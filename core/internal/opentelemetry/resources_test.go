package opentelemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestCreateResource_K8sAttributes(t *testing.T) {
	tests := []struct {
		name          string
		envVars       map[string]string
		expectedAttrs map[string]string
	}{
		{
			name: "all three k8s env vars set",
			envVars: map[string]string{
				"CONTAINER_NAME": "test-container",
				"POD_NAME":       "test-pod",
				"NAMESPACE_NAME": "test-namespace",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SContainerNameKey): "test-container",
				string(semconv.K8SPodNameKey):       "test-pod",
				string(semconv.K8SNamespaceNameKey): "test-namespace",
			},
		},
		{
			name: "only container name set",
			envVars: map[string]string{
				"CONTAINER_NAME": "test-container",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SContainerNameKey): "test-container",
			},
		},
		{
			name: "only pod name set",
			envVars: map[string]string{
				"POD_NAME": "test-pod",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SPodNameKey): "test-pod",
			},
		},
		{
			name: "only namespace name set",
			envVars: map[string]string{
				"NAMESPACE_NAME": "test-namespace",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SNamespaceNameKey): "test-namespace",
			},
		},
		{
			name: "two env vars set - container and pod",
			envVars: map[string]string{
				"CONTAINER_NAME": "test-container",
				"POD_NAME":       "test-pod",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SContainerNameKey): "test-container",
				string(semconv.K8SPodNameKey):       "test-pod",
			},
		},
		{
			name: "two env vars set - pod and namespace",
			envVars: map[string]string{
				"POD_NAME":       "test-pod",
				"NAMESPACE_NAME": "test-namespace",
			},
			expectedAttrs: map[string]string{
				string(semconv.K8SPodNameKey):       "test-pod",
				string(semconv.K8SNamespaceNameKey): "test-namespace",
			},
		},
		{
			name:          "no k8s env vars set",
			envVars:       map[string]string{},
			expectedAttrs: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the env vars for this test using t.Setenv for automatic cleanup
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create the resource
			ctx := context.Background()
			res, err := createResource(ctx)
			require.NoError(t, err)
			require.NotNil(t, res)

			// Verify expected attributes are present with correct values
			verifyExpectedAttributes(t, res, tt.expectedAttrs)
		})
	}
}

func verifyExpectedAttributes(
	t *testing.T,
	res interface{ Attributes() []attribute.KeyValue },
	expectedAttrs map[string]string,
) {
	t.Helper()

	attrs := res.Attributes()

	// Check that expected attributes are present with correct values
	for expectedKey, expectedValue := range expectedAttrs {
		found := false
		for _, attr := range attrs {
			if string(attr.Key) == expectedKey {
				found = true
				assert.Equal(t, expectedValue, attr.Value.AsString(),
					"attribute %s should have value %s", expectedKey, expectedValue)

				break
			}
		}
		assert.True(t, found, "expected attribute %s not found in resource", expectedKey)
	}

	// Verify that k8s attributes we didn't set are not present
	for _, attr := range attrs {
		attrKey := string(attr.Key)
		if isK8sAttribute(attrKey) {
			_, shouldBePresent := expectedAttrs[attrKey]
			assert.True(t, shouldBePresent,
				"attribute %s is present but should not be", attrKey)
		}
	}
}

func isK8sAttribute(key string) bool {
	return key == string(semconv.K8SContainerNameKey) ||
		key == string(semconv.K8SPodNameKey) ||
		key == string(semconv.K8SNamespaceNameKey)
}

func TestCreateResource_WithOTELResourceAttributes(t *testing.T) {
	// Test that k8s attributes are merged correctly with OTEL_RESOURCE_ATTRIBUTES
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=1.0.0,deployment.environment=test")
	t.Setenv("CONTAINER_NAME", "my-container")
	t.Setenv("POD_NAME", "my-pod")

	ctx := context.Background()
	res, err := createResource(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify all expected attributes are present
	expectedAttrs := map[string]string{
		"service.version":                   "1.0.0",
		"deployment.environment":            "test",
		string(semconv.K8SContainerNameKey): "my-container",
		string(semconv.K8SPodNameKey):       "my-pod",
	}

	verifyExpectedAttributes(t, res, expectedAttrs)
}

func TestCreateResource_NoOTELConfig(t *testing.T) {
	// Test that resource creation works even when no OTEL env vars are set
	// and k8s env vars are present
	t.Setenv("CONTAINER_NAME", "standalone-container")

	ctx := context.Background()
	res, err := createResource(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)

	attrs := res.Attributes()
	foundContainerName := false

	for _, attr := range attrs {
		if string(attr.Key) == string(semconv.K8SContainerNameKey) {
			foundContainerName = true
			assert.Equal(t, "standalone-container", attr.Value.AsString())
			break
		}
	}

	assert.True(t, foundContainerName, "k8s.container.name should be present even without OTEL_* env vars")
}

func TestCreateResource_UserDefinedK8sAttributesNotOverridden(t *testing.T) {
	// Test that user-defined k8s attributes are NOT overridden by env vars
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "k8s.container.name=user-container,k8s.pod.name=user-pod")
	t.Setenv("CONTAINER_NAME", "env-container")
	t.Setenv("POD_NAME", "env-pod")
	t.Setenv("NAMESPACE_NAME", "env-namespace")

	ctx := context.Background()
	res, err := createResource(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)

	attrs := res.Attributes()

	foundAttrs := extractK8sAttributes(attrs)

	// User-defined values should be preserved
	assert.Equal(t, "user-container", foundAttrs[semconv.K8SContainerNameKey],
		"k8s.container.name should use user-defined value, not env var")
	assert.Equal(t, "user-pod", foundAttrs[semconv.K8SPodNameKey],
		"k8s.pod.name should use user-defined value, not env var")
	// Missing user-defined value should be added from env var
	assert.Equal(t, "env-namespace", foundAttrs[semconv.K8SNamespaceNameKey],
		"k8s.namespace.name should use env var value since not user-defined")
}

func TestCreateResource_PartialUserDefinedK8sAttributes(t *testing.T) {
	// Test that we add missing k8s attributes even when some are user-defined
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "k8s.container.name=user-container")
	t.Setenv("CONTAINER_NAME", "env-container")
	t.Setenv("POD_NAME", "env-pod")
	t.Setenv("NAMESPACE_NAME", "env-namespace")

	ctx := context.Background()
	res, err := createResource(ctx)
	require.NoError(t, err)
	require.NotNil(t, res)

	foundAttrs := extractK8sAttributes(res.Attributes())

	// Container should use user-defined value
	assert.Equal(t, "user-container", foundAttrs[semconv.K8SContainerNameKey],
		"k8s.container.name should use user-defined value")

	// Pod and namespace should use env var values since not user-defined
	assert.Equal(t, "env-pod", foundAttrs[semconv.K8SPodNameKey],
		"k8s.pod.name should use env var value since not user-defined")
	assert.Equal(t, "env-namespace", foundAttrs[semconv.K8SNamespaceNameKey],
		"k8s.namespace.name should use env var value since not user-defined")
}

// extractK8sAttributes extracts all k8s attributes from a slice of attributes.
func extractK8sAttributes(attrs []attribute.KeyValue) map[attribute.Key]string {
	found := make(map[attribute.Key]string)
	for _, attr := range attrs {
		if isK8sAttribute(string(attr.Key)) {
			found[attr.Key] = attr.Value.AsString()
		}
	}

	return found
}

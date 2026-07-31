/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package opentelemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

func TestLookupK8sAttrs(t *testing.T) {
	t.Run("returns nil when no env vars are set", func(t *testing.T) {
		assert.Empty(t, lookupK8sAttrs())
	})

	t.Run("returns all three attributes when all env vars are set", func(t *testing.T) {
		t.Setenv("POD_NAME", "test-pod")
		t.Setenv("NAMESPACE_NAME", "test-namespace")
		t.Setenv("CONTAINER_NAME", "test-container")

		attrs := lookupK8sAttrs()
		require.Len(t, attrs, 3)

		m := attrsToMap(attrs)
		assert.Equal(t, "test-pod", m[string(semconv.K8SPodNameKey)])
		assert.Equal(t, "test-namespace", m[string(semconv.K8SNamespaceNameKey)])
		assert.Equal(t, "test-container", m[string(semconv.K8SContainerNameKey)])
	})

	t.Run("returns only set attributes", func(t *testing.T) {
		t.Setenv("POD_NAME", "my-pod")

		attrs := lookupK8sAttrs()
		require.Len(t, attrs, 1)

		m := attrsToMap(attrs)
		assert.Equal(t, "my-pod", m[string(semconv.K8SPodNameKey)])
	})
}

func TestBuildResource(t *testing.T) {
	t.Run("includes k8s attributes from env", func(t *testing.T) {
		t.Setenv("POD_NAME", "test-pod")
		t.Setenv("NAMESPACE_NAME", "test-ns")
		t.Setenv("CONTAINER_NAME", "manager")

		res, err := buildResource(context.Background())
		require.NoError(t, err)

		m := resourceAttrsToMap(res)
		assert.Equal(t, "test-pod", m[string(semconv.K8SPodNameKey)])
		assert.Equal(t, "test-ns", m[string(semconv.K8SNamespaceNameKey)])
		assert.Equal(t, "manager", m[string(semconv.K8SContainerNameKey)])
	})

	t.Run("works with no env vars at all", func(t *testing.T) {
		res, err := buildResource(context.Background())
		require.NoError(t, err)
		require.NotNil(t, res)
	})

	t.Run("merges OTEL resource attributes with k8s attrs", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=1.0.0")
		t.Setenv("POD_NAME", "test-pod")

		res, err := buildResource(context.Background())
		require.NoError(t, err)

		m := resourceAttrsToMap(res)
		assert.Equal(t, "1.0.0", m["service.version"])
		assert.Equal(t, "test-pod", m[string(semconv.K8SPodNameKey)])
	})

	t.Run("user-defined k8s attributes take precedence over downward API", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "k8s.pod.name=user-pod")
		t.Setenv("POD_NAME", "downward-api-pod")

		res, err := buildResource(context.Background())
		require.NoError(t, err)

		m := resourceAttrsToMap(res)
		assert.Equal(t, "user-pod", m[string(semconv.K8SPodNameKey)])
	})

	t.Run("has a valid schema URL", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "klio-operator")

		res, err := buildResource(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, res.SchemaURL())
	})

	t.Run("resource can be merged with another SDK resource", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "klio-operator")

		res, err := buildResource(context.Background())
		require.NoError(t, err)

		other, err := resource.New(context.Background(),
			resource.WithAttributes(semconv.ServiceVersion("2.0.0")),
			resource.WithSchemaURL(semconv.SchemaURL),
		)
		require.NoError(t, err)

		merged, err := resource.Merge(res, other)
		require.NoError(t, err)

		m := resourceAttrsToMap(merged)
		assert.Equal(t, "klio-operator", m[string(semconv.ServiceNameKey)])
		assert.Equal(t, "2.0.0", m[string(semconv.ServiceVersionKey)])
	})
}

func TestDetectAdditionalResources(t *testing.T) {
	t.Run("returns nil when env var is not set", func(t *testing.T) {
		res, err := detectAdditionalResources(context.Background())
		require.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("returns nil for empty env var", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_DETECTORS", "")

		res, err := detectAdditionalResources(context.Background())
		require.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("returns nil for whitespace-only env var", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_DETECTORS", " , , ")

		res, err := detectAdditionalResources(context.Background())
		require.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("returns error for unknown detector", func(t *testing.T) {
		t.Setenv("OTEL_RESOURCE_DETECTORS", "nonexistent")

		_, err := detectAdditionalResources(context.Background())
		assert.Error(t, err)
	})
}

func attrsToMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.AsString()
	}

	return m
}

func resourceAttrsToMap(res *resource.Resource) map[string]string {
	return attrsToMap(res.Attributes())
}

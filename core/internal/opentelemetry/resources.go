package opentelemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/contrib/detectors/autodetect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// createResource creates an OpenTelemetry resource with automatic detection support.
func createResource(ctx context.Context) (*resource.Resource, error) {
	// Start with basic resource configuration
	res, err := resource.New(ctx,
		// Environment variables (OTEL_RESOURCE_ATTRIBUTES, OTEL_SERVICE_NAME, etc.)
		resource.WithFromEnv(),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base resource: %w", err)
	}

	// Apply auto-detection if configured
	if detectedRes, err := detectAdditionalResources(ctx); err != nil {
		log.FromContext(ctx).Warning("failed to detect additional resources", "err", err)
	} else if detectedRes != nil {
		res, err = resource.Merge(res, detectedRes)
		if err != nil {
			return nil, fmt.Errorf("failed to merge resources: %w", err)
		}
	}

	// Add Kubernetes container information if available
	if k8sRes := buildK8sResource(ctx); k8sRes != nil {
		res, err = resource.Merge(k8sRes, res)
		if err != nil {
			return nil, fmt.Errorf("failed to merge k8s resource: %w", err)
		}
	}

	return res, nil
}

// buildK8sResource builds a resource with Kubernetes container information from environment variables.
// It looks for CONTAINER_NAME, POD_NAME, and NAMESPACE_NAME environment variables.
// Each attribute is added independently if its environment variable is present.
// User-defined values in the existing resource take precedence over these defaults (via reverse merge).
// Returns nil if no K8s environment variables are set.
func buildK8sResource(ctx context.Context) *resource.Resource {
	var k8sAttrs []attribute.KeyValue
	logger := log.FromContext(ctx)

	if containerName, ok := os.LookupEnv("CONTAINER_NAME"); ok {
		k8sAttrs = append(k8sAttrs, semconv.K8SContainerNameKey.String(containerName))
		logger.Debug("Adding k8s.container.name resource attribute", "containerName", containerName)
	}
	if podName, ok := os.LookupEnv("POD_NAME"); ok {
		k8sAttrs = append(k8sAttrs, semconv.K8SPodNameKey.String(podName))
		logger.Debug("Adding k8s.pod.name resource attribute", "podName", podName)
	}
	if namespaceName, ok := os.LookupEnv("NAMESPACE_NAME"); ok {
		k8sAttrs = append(k8sAttrs, semconv.K8SNamespaceNameKey.String(namespaceName))
		logger.Debug("Adding k8s.namespace.name resource attribute", "namespaceName", namespaceName)
	}

	if len(k8sAttrs) == 0 {
		return nil
	}

	k8sRes, err := resource.New(ctx,
		resource.WithAttributes(k8sAttrs...),
	)
	if err != nil {
		logger.Error(err, "failed to create k8s resource")
		return nil
	}

	return k8sRes
}

// detectAdditionalResources detects additional resource attributes using the configured detectors.
// The detectors are specified in the OTEL_RESOURCE_DETECTORS environment variable as a comma-separated list.
// The available detectors are defined in the "go.opentelemetry.io/contrib/detectors/autodetect" package.
func detectAdditionalResources(ctx context.Context) (*resource.Resource, error) {
	detectors, ok := os.LookupEnv(resourceDetectorEnvVar)
	if !ok || detectors == "" {
		return nil, nil
	}

	names := strings.Split(detectors, ",")
	ids := make([]autodetect.ID, 0, len(names))
	for _, name := range names {
		if trimmedName := strings.TrimSpace(name); trimmedName != "" {
			ids = append(ids, autodetect.ID(trimmedName))
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}

	detector, err := autodetect.Detector(ids...)
	if err != nil {
		return nil, fmt.Errorf("failed to create detector for %v: %w", ids, err)
	}

	return detector.Detect(ctx)
}

package opentelemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/contrib/detectors/autodetect"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
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

	return res, nil
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

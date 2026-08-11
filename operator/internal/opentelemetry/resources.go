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
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/detectors/autodetect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// buildResource builds the OpenTelemetry resource for the operator.
// It starts from OTEL_* env vars (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, ...),
// applies any detectors named in OTEL_RESOURCE_DETECTORS, and layers in k8s.*
// attributes derived from the downward-API env vars (POD_NAME / NAMESPACE_NAME /
// CONTAINER_NAME) that the Helm chart injects. Attributes set via env win, so
// users can still override.
func buildResource(ctx context.Context) (*resource.Resource, error) {
	envRes, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base resource: %w", err)
	}

	if detectedRes, err := detectAdditionalResources(ctx); err != nil {
		logger.Info("failed to detect additional resources", "err", err)
	} else if detectedRes != nil {
		// Schema-URL conflicts (common when detector contrib and our pinned
		// semconv import drift) are non-fatal: keep the env-only resource and
		// warn rather than aborting OTel setup over an optional detector.
		merged, mergeErr := resource.Merge(envRes, detectedRes)
		if mergeErr != nil {
			logger.Info("ignoring detected resources due to merge error", "err", mergeErr)
		} else {
			envRes = merged
		}
	}

	k8sAttrs := lookupK8sAttrs()
	if len(k8sAttrs) == 0 {
		return envRes, nil
	}

	k8sRes, err := resource.New(ctx, resource.WithAttributes(k8sAttrs...))
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s resource: %w", err)
	}

	merged, err := resource.Merge(k8sRes, envRes)
	if err != nil {
		return nil, fmt.Errorf("failed to merge k8s resource: %w", err)
	}

	return merged, nil
}

// lookupK8sAttrs reads the downward-API env vars the Helm chart injects and
// returns a slice of OTel resource attributes for whichever are set.
func lookupK8sAttrs() []attribute.KeyValue {
	var attrs []attribute.KeyValue
	if v, ok := os.LookupEnv("POD_NAME"); ok {
		attrs = append(attrs, semconv.K8SPodNameKey.String(v))
	}
	if v, ok := os.LookupEnv("NAMESPACE_NAME"); ok {
		attrs = append(attrs, semconv.K8SNamespaceNameKey.String(v))
	}
	if v, ok := os.LookupEnv("CONTAINER_NAME"); ok {
		attrs = append(attrs, semconv.K8SContainerNameKey.String(v))
	}

	return attrs
}

// detectAdditionalResources runs the detectors listed (comma-separated) in
// OTEL_RESOURCE_DETECTORS. Detector IDs are defined in the
// "go.opentelemetry.io/contrib/detectors/autodetect" package.
func detectAdditionalResources(ctx context.Context) (*resource.Resource, error) {
	detectors, ok := os.LookupEnv("OTEL_RESOURCE_DETECTORS")
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

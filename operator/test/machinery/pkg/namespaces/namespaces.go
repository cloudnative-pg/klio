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

// Package namespaces provides helpers to inspect the objects living in a
// Kubernetes namespace during e2e tests.
package namespaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

// extractNamespaceObjects lists the given object lists in the namespace and
// returns the indented JSON representation of each list keyed by its list kind
// (for example "PodList" or "ClusterList"). It is intended to capture cluster
// state for debugging e2e test failures, before the namespace is torn down.
// Errors listing or marshaling individual kinds are aggregated and returned so
// that a single failing kind does not prevent the others from being extracted.
func extractNamespaceObjects(
	ctx context.Context, r *resources.Resources, namespace string, lists []k8s.ObjectList,
) (map[string][]byte, error) {
	nsResources := r.WithNamespace(namespace)
	scheme := nsResources.GetScheme()
	objects := make(map[string][]byte)
	var errs []error
	for _, list := range lists {
		// The typed client does not populate TypeMeta, so the kind is resolved
		// from the scheme; it names both the error messages and the output file.
		gvks, _, err := scheme.ObjectKinds(list)
		if err != nil || len(gvks) == 0 {
			errs = append(errs, fmt.Errorf("looking up kind for %T: %w", list, err))
			continue
		}
		gvk := gvks[0]

		if err := nsResources.List(ctx, list); err != nil {
			errs = append(errs, fmt.Errorf("listing %s in namespace %q: %w", gvk.Kind, namespace, err))
			continue
		}

		// Stamp the kind back so the marshaled output also carries kind/apiVersion.
		list.GetObjectKind().SetGroupVersionKind(gvk)

		out, err := json.MarshalIndent(list, "", "    ")
		if err != nil {
			errs = append(errs, fmt.Errorf("marshaling %s: %w", gvk.Kind, err))
			continue
		}
		objects[gvk.Kind] = out
	}

	return objects, errors.Join(errs...)
}

// writeNamespaceObjects writes each entry returned by extractNamespaceObjects to
// its own "<kind>.json" file inside dir. Write errors are aggregated so that a
// single failing file does not prevent the others from being written.
func writeNamespaceObjects(dir string, objects map[string][]byte) error {
	var errs []error
	for kind, data := range objects {
		path := filepath.Join(dir, kind+".json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			errs = append(errs, fmt.Errorf("writing %q: %w", path, err))
		}
	}

	return errors.Join(errs...)
}

// DumpNamespaceOnFailure writes a snapshot of the given object lists in the
// namespace, one "<kind>.json" file per resource kind, under
// "<logDir>/<namespace>" when the test has failed. It is a no-op for passing
// tests and never fails the test itself: dump errors are only logged, so a dump
// failure does not mask the original test failure.
func DumpNamespaceOnFailure(
	ctx context.Context, t *testing.T, r *resources.Resources, logDir, namespace string, lists []k8s.ObjectList,
) {
	t.Helper()
	if !t.Failed() {
		return
	}

	// The path is built from test-controlled configuration, not user input.
	dir := filepath.Join(logDir, namespace)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Logf("failed to create namespace dump directory %q: %s", dir, err)
		return
	}

	objects, err := extractNamespaceObjects(ctx, r, namespace, lists)
	if err != nil {
		t.Logf("failed to extract objects for namespace %q: %s", namespace, err)
	}
	if err := writeNamespaceObjects(dir, objects); err != nil {
		t.Logf("failed to write objects for namespace %q: %s", namespace, err)
		return
	}
	t.Logf("dumped objects for namespace %q to %s", namespace, dir)
}

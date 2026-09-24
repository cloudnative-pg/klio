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

package conditions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
)

// tier2AnnotationName is the annotation key used to mark backups present in
// tier2 storage.
const tier2AnnotationName = "klio.io/tier2"

// presentAnnotationValue is the value set when a backup is present in a tier.
const presentAnnotationValue = "present"

// backupMetadata is the subset of `klio admin list-backups` JSON output
// needed to count backups.
type backupMetadata struct {
	Name        string            `json:"name"`
	ClusterName string            `json:"clusterName"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// BackupCountEquals checks if `klio admin list-backups`, run in the given
// Klio server pod, reports exactly expectedCount backups.
func BackupCountEquals(
	r *resources.Resources,
	namespace, podName, containerName string,
	expectedCount int,
) wait.ConditionWithContextFunc {
	return backupCountMatches(r, namespace, podName, containerName, expectedCount, func(backupMetadata) bool {
		return true
	})
}

// Tier2BackupCountEquals checks if `klio admin list-backups`, run in the
// given Klio server pod, reports exactly expectedCount backups present in
// tier2 storage (marked with the tier2 annotation).
func Tier2BackupCountEquals(
	r *resources.Resources,
	namespace, podName, containerName string,
	expectedCount int,
) wait.ConditionWithContextFunc {
	return backupCountMatches(r, namespace, podName, containerName, expectedCount, func(b backupMetadata) bool {
		return b.Annotations[tier2AnnotationName] == presentAnnotationValue
	})
}

// backupCountMatches lists backups via `klio admin list-backups` and checks
// that exactly expectedCount of them match the given predicate. It returns
// (false, nil) on transient errors, so the wait keeps retrying.
func backupCountMatches(
	r *resources.Resources,
	namespace, podName, containerName string,
	expectedCount int,
	predicate func(backupMetadata) bool,
) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		var stdout, stderr bytes.Buffer
		cmd := []string{"klio", "admin", "list-backups"}
		if err := r.ExecInPod(ctx, namespace, podName, containerName, cmd, &stdout, &stderr); err != nil {
			return false, nil //nolint:nilerr // keep retrying on transient failures
		}

		var backups []backupMetadata
		if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
			return false, nil //nolint:nilerr // keep retrying on transient failures
		}

		count := 0
		for _, b := range backups {
			if predicate(b) {
				count++
			}
		}

		return count == expectedCount, nil
	}
}

// PluginConfigurationHasCondition checks if the PluginConfiguration has the specified condition
// with the expected status and observed generation.
func PluginConfigurationHasCondition(
	r *resources.Resources,
	pc k8s.Object,
	conditionType string,
	expectedStatus metav1.ConditionStatus,
	expectedGeneration int64,
) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, pc.GetName(), pc.GetNamespace(), pc); err != nil {
			return false, fmt.Errorf("failed to get PluginConfiguration: %w", err)
		}

		pluginConfig, ok := pc.(*kliov1alpha1.PluginConfiguration)
		if !ok {
			return false, fmt.Errorf("object is not a PluginConfiguration: %v", pc)
		}

		for _, condition := range pluginConfig.Status.Conditions {
			if condition.Type == conditionType {
				statusMatch := condition.Status == expectedStatus
				generationMatch := condition.ObservedGeneration == expectedGeneration
				return statusMatch && generationMatch, nil
			}
		}

		return false, nil
	}
}

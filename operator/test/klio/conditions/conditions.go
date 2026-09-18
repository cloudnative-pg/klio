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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	waitConditions "sigs.k8s.io/e2e-framework/klient/wait/conditions"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/klio/podexec"
)

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

// KlioServerIsReady checks if the given KlioServer is ready by checking the readiness of its pod.
func KlioServerIsReady(r *resources.Resources, server k8s.Object) wait.ConditionWithContextFunc {
	// TODO: This is a temporary solution, we should use the KlioServer controller to manage the readiness of the server.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.GetName() + "-klio-0",
			Namespace: server.GetNamespace(),
		},
	}

	return waitConditions.New(r).PodReady(pod)
}

// CheckTierHasBackups checks if the tier identified by tierAnnotation has
// exactly the expected number of backups. Returns (false, nil) on transient
// errors to allow the wait to continue retrying.
func CheckTierHasBackups(
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
	tierAnnotation string,
	expectedCount int,
) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		names, err := podexec.ListTierBackupNames(ctx, r, namespace, serverName, clusterName, tierAnnotation)
		if err != nil {
			// Return false without error to keep retrying on transient failures.
			return false, nil //nolint:nilerr
		}

		return len(names) == expectedCount, nil
	}
}

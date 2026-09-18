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

package podexec

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
)

const (
	// klioPodSuffix is the suffix added to a Klio server name to form its pod name.
	klioPodSuffix = "-klio-0"

	// serverContainerName is the klio server pod's container that runs klio admin commands.
	serverContainerName = "server"

	// presentAnnotationValue is the value set on a Backup when it is present in a tier.
	presentAnnotationValue = "present"
)

// ListTierBackupNames returns the names of the backups currently present in
// the tier identified by tierAnnotation for the given cluster, ordered
// newest first by their start time.
func ListTierBackupNames(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	serverName string,
	clusterName string,
	tierAnnotation string,
) ([]string, error) {
	podName := serverName + klioPodSuffix

	// Use the Klio admin API to list backups.
	var stdout, stderr bytes.Buffer
	klioCmd := []string{"klio", "admin", "list-backups"}
	if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("failed to list backups: %w; stderr: %s", err, stderr.String())
	}

	type backupMetadata struct {
		Name        string            `json:"name"`
		ClusterName string            `json:"clusterName"`
		StartedAt   int64             `json:"startedAt"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}

	var backups []backupMetadata
	if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
		return nil, fmt.Errorf("failed to parse backup list: %w", err)
	}

	slices.SortFunc(backups, func(a, b backupMetadata) int {
		return cmp.Compare(b.StartedAt, a.StartedAt)
	})

	// This cluster's backups present in the tier (those carrying the tier
	// annotation).
	names := make([]string, 0, len(backups))
	for i := range backups {
		if backups[i].ClusterName != clusterName {
			continue
		}
		if backups[i].Annotations[tierAnnotation] == presentAnnotationValue {
			names = append(names, backups[i].Name)
		}
	}

	return names, nil
}

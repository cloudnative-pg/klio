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

package mutators

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// CreateBackupIDMutator creates a mutator function that updates the Klio PluginConfiguration
// for recovery with the BackupID from the specified backup.
func CreateBackupIDMutator(
	backup *cnpgv1.Backup,
) machineryFeatures.RecoveryClusterMutateFunc {
	return func(ctx context.Context, cluster *cnpgv1.Cluster, r *resources.Resources) error {
		// Get the backupID from the created backup
		backupRes := &cnpgv1.Backup{}
		err := r.Get(ctx, backup.Name, backup.Namespace, backupRes)
		if err != nil {
			return fmt.Errorf("failed to get backup: %w", err)
		}

		// Extract and validate the backupID
		backupID := backupRes.Status.BackupID
		if backupID == "" {
			return fmt.Errorf("backup %s/%s has an empty BackupID in its status", backup.Namespace, backup.Name)
		}

		cluster.Spec.Bootstrap.Recovery.RecoveryTarget = &cnpgv1.RecoveryTarget{
			TargetImmediate: new(true),
			BackupID:        backupID,
		}

		return nil
	}
}

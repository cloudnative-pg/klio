package mutators

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"

	"github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// CreateBackupIDMutator creates a mutator function that updates the Klio PluginConfiguration
// for recovery with the BackupID from the specified backup.
func CreateBackupIDMutator(
	backup *cnpgv1.Backup,
	pluginConfigName, pluginConfigNamespace string,
) machineryFeatures.RecoveryClusterMutateFunc {
	return func(ctx context.Context, _ *cnpgv1.Cluster, r *resources.Resources) error {
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

		// Update the Klio PluginConfiguration for recovery with the BackupID
		pluginConfiguration := &v1alpha1.PluginConfiguration{}
		err = r.Get(ctx, pluginConfigName, pluginConfigNamespace, pluginConfiguration)
		if err != nil {
			return fmt.Errorf("failed to get PluginConfiguration: %w", err)
		}

		// Update the BackupID in place
		pluginConfiguration.Spec.BackupID = backupID
		err = r.Update(ctx, pluginConfiguration)
		if err != nil {
			return fmt.Errorf("failed to update Klio PluginConfiguration for recovery: %w", err)
		}

		// TODO: Uncomment once the backupID field is supported in the RecoveryTarget
		// Set the RecoveryTarget in the recovery Cluster
		// cluster.Spec.Bootstrap.Recovery.RecoveryTarget = &cnpgv1.RecoveryTarget{
		// 	TargetImmediate: ptr.To(true),
		// 	BackupID:        backupID,
		// }

		return nil
	}
}

package mutators

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"k8s.io/utils/ptr"
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
			TargetImmediate: ptr.To(true),
			BackupID:        backupID,
		}

		return nil
	}
}

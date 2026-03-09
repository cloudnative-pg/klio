package e2e

import (
	"context"
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/runner"
)

func TestMain(m *testing.M) {
	runner.RegisterFeature(BackupFromPrimary(envconf.RandomName("backup-from-primary", 32)))
	runner.RegisterFeature(BackupFromStandby(envconf.RandomName("backup-from-standby", 32)))
	runner.RegisterFeature(RecoverClusterFromBackupID(envconf.RandomName("recovery-from-backup-id", 32)))
	runner.RegisterFeature(RecoverClusterFromLatestBackup(envconf.RandomName("recovery-from-latest-backup", 32)))
	runner.RegisterFeature(RecoverReplicaCluster(envconf.RandomName("recovery-replica-cluster", 32)))
	runner.RegisterFeature(RecoverClusterWithTablespaces(envconf.RandomName("recovery-tablespace", 32)))
	runner.RegisterFeature(RecoverClusterFromPitr(envconf.RandomName("recovery-from-pitr", 32)))
	runner.RegisterFeature(RecoverClusterFromTier2(envconf.RandomName("recovery-from-tier2", 32)))
	runner.RegisterFeature(RecoverClusterFromTier2Pitr(envconf.RandomName("recovery-from-tier2-pitr", 32)))
	runner.RegisterFeature(PluginConfigurationUpdate(envconf.RandomName("plugin-config-update", 32)))
	runner.RegisterFeature(Tier2Retention(envconf.RandomName("tier2-retention", 32)))
	runner.RegisterFeature(WALRetentionQueueAwareness(envconf.RandomName("wal-retention-queue", 32)))
	runner.RegisterFeature(ServerTierReconfiguration(envconf.RandomName("server-reconfig", 32)))
	runner.RegisterSetup(
		func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			if err := certmanagerv1.AddToScheme(config.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}
			if err := kliov1alpha1.AddToScheme(config.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
	)

	runner.RunMain(m)
}

func TestFeatures(t *testing.T) {
	runner.RunAllFeatures(t)
}

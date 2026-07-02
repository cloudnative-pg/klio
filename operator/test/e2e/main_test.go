package e2e

import (
	"context"
	"log"
	"testing"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/sternmultitailer"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/runner"
)

//nolint:gochecknoglobals // testCfg is a global variable that holds the test configuration loaded in TestMain.
var testCfg *testconfig.Config

func TestMain(m *testing.M) {
	var err error
	testCfg, err = testconfig.Load()
	if err != nil {
		log.Fatalf("failed to load test config: %s", err)
	}

	var sternCancel context.CancelFunc
	var sternDoneChs []chan struct{}

	runner.RegisterFeature(BackupFromPrimary(envconf.RandomName("backup-from-primary", 32)))
	runner.RegisterFeature(BackupFromStandby(envconf.RandomName("backup-from-standby", 32)))
	runner.RegisterFeature(Tier1ServerSideMaintenance(envconf.RandomName("tier1-maintenance", 32)))
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
	runner.RegisterFeature(PVCResize(envconf.RandomName("pvc-resize", 32)))
	runner.RegisterFeature(OTELMetricsAndTraces(envconf.RandomName("otel", 32)))
	runner.RegisterFeature(
		OperatorOTELMetrics(
			envconf.RandomName("operator-otel", 32), testCfg.OperatorNamespace,
			testCfg.OperatorAppLabel, testCfg.OperatorSubscription),
		runner.WithSerialExecution(),
	)
	runner.RegisterFeature(MissingPluginConfiguration(envconf.RandomName("missing-plugin-config", 32)))

	runner.RegisterSetup(
		func(ctx context.Context, config *envconf.Config) (context.Context, error) {
			if err := certmanagerv1.AddToScheme(config.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}
			if err := kliov1alpha1.AddToScheme(config.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}

			logDir := testCfg.LogDir
			client, err := kubernetes.NewForConfig(config.Client().RESTConfig())
			if err != nil {
				return ctx, err
			}

			// Disable linter and SonarQube: cancel is stored in sternCancel and called in teardown
			var sternCtx context.Context
			sternCtx, sternCancel = context.WithCancel(ctx) //nolint:gosec
			labelSelectors := []labels.Set{
				{"app.kubernetes.io/name": "klio"},
				{"app.kubernetes.io/name": "cloudnative-pg"},
				{"app.kubernetes.io/name": "postgresql"},
				{"app.kubernetes.io/instance": "rustfs"},
				{"batch.kubernetes.io/job-name": "rustfs"},
				{"klio.cnpg.io/type": "base"},
			}
			for _, ls := range labelSelectors {
				sternDoneChs = append(sternDoneChs,
					sternmultitailer.StreamLogs(sternCtx, client,
						labels.SelectorFromSet(ls), logDir))
			}

			return ctx, nil
		},
	)
	runner.RegisterTeardown(
		func(ctx context.Context, _ *envconf.Config) (context.Context, error) {
			sternCancel()
			for _, ch := range sternDoneChs {
				<-ch
			}

			return ctx, nil
		},
	)

	runner.RunMain(m)
}

func TestFeatures(t *testing.T) {
	runner.RunAllFeatures(t)
}

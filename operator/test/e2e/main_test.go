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
	runner.RegisterFeatures(RecoverClusterFromBackup(envconf.RandomName("recovery-from-backup", 32)))
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

package e2e

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/secrets"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates"
)

func NewRecoveryFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.RecoveryFeatureConfig {
	const klioServerName = "test-klio-server"
	var sourcePrimary corev1.Pod

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	clientSecret := secrets.GetKlioClientSecret("klio-client", namespace, "klio", "testclientpassword123")
	cnpgCluster := templates.GetCnpgClusterObject("test-cluster-source", namespace, instances, certificate,
		clientSecret)

	userSecret := secrets.GetKlioUsersSecret("test-user", namespace, clientSecret, cnpgCluster.Name)
	encryptionSecret := secrets.GetKlioEncryptionSecret("encryption", namespace, "testencryptionpassword123")
	klioServer := templates.GetKlioServerObject(klioServerName, namespace, certificate.Spec.SecretName,
		encryptionSecret, userSecret)

	backup := templates.GetCnpgBackupObject("test-backup", namespace, cnpgv1.DefaultBackupTarget, cnpgCluster)

	// Generate the recovery Cluster object
	recoveryCluster := cnpgCluster.DeepCopy()
	recoveryCluster.Name = "test-cluster-restored"
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: "source-cluster",
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: ptr.To(true),
			Parameters: map[string]string{
				"serverAddress":    certificate.Spec.DNSNames[0],
				"clientSecretName": clientSecret.GetName(),
				"serverSecretName": certificate.Spec.SecretName,
				"backupName":       backup.Name,
				"clusterName":      cnpgCluster.Name,
			},
		},
	}}
	recoveryCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: "source-cluster",
		},
	}
	// TODO: we should check that WAL archiving and backups also work in the recovered Cluster
	recoveryCluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	setupFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Creating resources for recovery feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Create(ctx, namespaceObj), "failed to create namespace")
		require.NoError(t, r.Create(ctx, clientSecret), "failed to create client secret")
		require.NoError(t, r.Create(ctx, cnpgCluster), "failed to create CNPG source Cluster")
		require.NoError(t, r.Create(ctx, userSecret), "failed to create user secret")
		require.NoError(t, r.Create(ctx, encryptionSecret), "failed to create encryption secret")
		require.NoError(t, r.Create(ctx, issuer), "failed to create issuer")
		require.NoError(t, r.Create(ctx, certificate), "failed to create certificate")
		require.NoError(t, r.Create(ctx, klioServer), "failed to create KLIO server")

		t.Logf("Waiting for resources to be ready for recovery feature: %s", name)
		err = wait.For(
			machineryConditions.ClusterIsReady(r, cnpgCluster),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for CNPG source Cluster to be ready")

		err = wait.For(
			conditions.KlioServerIsReady(r, klioServer),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for Klio server to be ready")

		require.NoError(t, r.Get(ctx, cnpgCluster.Status.CurrentPrimary, namespace, &sourcePrimary),
			"failed to get the current primary pod")

		t.Logf("Resources created and ready for recovery feature: %s", name)

		return ctx
	}

	teardownFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Tearing down resources for recovery feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Delete(ctx, namespaceObj), "failed to delete namespace")
		t.Logf("Resources torn down for recovery feature: %s", name)

		return ctx
	}

	return machineryFeatures.RecoveryFeatureConfig{
		Name:             name,
		Setup:            setupFunc,
		Teardown:         teardownFunc,
		SourcePrimaryPod: &sourcePrimary,
		Backup:           backup,
		RecoveryCluster:  recoveryCluster,
	}
}

func RecoverClusterFromBackup(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromBackup", 1, namespace))
}

func RecoverClusterFromPitr(namespace string) *machineryFeatures.PitrFeature {
	return machineryFeatures.NewPitrFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromPitr", 1, namespace))
}

package e2e

import (
	"context"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func newBackupFeature(
	name string, backupTarget cnpgv1.BackupTarget, instances int, namespace string,
) *machineryFeatures.BackupFeature {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	cnpgCluster := templates.GetCnpgClusterObject("test-cluster", namespace, instances, "klio-plugin-configuration")

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace, "klio-user@test-cluster", caIssuer)
	klioPluginConfiguration := templates.GetKlioPluginConfigurationObject(
		"klio-plugin-configuration", namespace, certificate, userCertificate)
	encryptionSecret := secrets.GetKlioEncryptionSecret("encryption", namespace, "testencryptionpassword123")
	klioServer := templates.GetKlioServerObject(
		klioServerName,
		namespace,
		templates.KlioServerTemplateOptions{
			TLSSecretName:        certificate.Spec.SecretName,
			ClientCASecretName:   caCertificate.Spec.SecretName,
			EncryptionSecretName: encryptionSecret.Name,
		},
	)

	backup := templates.GetCnpgBackupObject("test-backup", namespace, backupTarget, cnpgCluster)

	setupFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Creating resources for backup feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Create(ctx, namespaceObj), "failed to create namespace")
		require.NoError(t, r.Create(ctx, cnpgCluster), "failed to create CNPG cluster")
		require.NoError(t, r.Create(ctx, klioPluginConfiguration), "failed to create Klio plugin configuration")
		require.NoError(t, r.Create(ctx, encryptionSecret), "failed to create encryption secret")
		require.NoError(t, r.Create(ctx, issuer), "failed to create issuer")
		require.NoError(t, r.Create(ctx, caCertificate), "failed to create CA certificate")
		require.NoError(t, r.Create(ctx, caIssuer), "failed to create CA issuer")
		require.NoError(t, r.Create(ctx, certificate), "failed to create certificate")
		require.NoError(t, r.Create(ctx, userCertificate), "failed to create user certificate")
		require.NoError(t, r.Create(ctx, klioServer), "failed to create KLIO server")

		t.Logf("Waiting for resources to be ready for backup feature: %s", name)
		err = wait.For(
			machineryConditions.ClusterIsReady(r, cnpgCluster),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for CNPG cluster to be ready")

		err = wait.For(
			conditions.KlioServerIsReady(r, klioServer),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "failed to wait for Klio server to be ready")
		t.Logf("Resources created and ready for backup feature: %s", name)

		return ctx
	}

	teardownFunc := func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Logf("Tearing down resources for backup feature: %s", name)
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")
		require.NoError(t, r.Delete(ctx, namespaceObj), "failed to delete namespace")
		t.Logf("Resources torn down for backup feature: %s", name)

		return ctx
	}

	return machineryFeatures.NewBackupFeature(machineryFeatures.BackupFeatureConfig{
		Name:     name,
		Setup:    setupFunc,
		Teardown: teardownFunc,
		Backup:   backup,
	})
}

func BackupFromPrimary(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromPrimary", cnpgv1.BackupTargetPrimary, 1, namespace)
}

func BackupFromStandby(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromStandby", cnpgv1.BackupTargetStandby, 2, namespace)
}

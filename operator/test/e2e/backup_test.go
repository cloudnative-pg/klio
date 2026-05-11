package e2e

import (
	"context"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/pods"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// assertVerificationRan checks that backup verification ran by looking for the log message.
func assertVerificationRan(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	backup *cnpgv1.Backup,
) {
	t.Helper()

	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Get the updated backup to access the instance ID
	var updatedBackup cnpgv1.Backup
	require.NoError(t, r.Get(ctx, backup.Name, backup.Namespace, &updatedBackup),
		"failed to get backup")
	require.NotNil(t, updatedBackup.Status.InstanceID, "backup instance ID should be set")

	// Get the pod that ran the backup
	var pod corev1.Pod
	require.NoError(t, r.Get(ctx, updatedBackup.Status.InstanceID.PodName, backup.Namespace, &pod),
		"failed to get backup pod")

	// Get logs from the klio-plugin container
	logs, err := pods.GetLogs(ctx, r, &pod, cnpgi.KlioPluginContainerName)
	require.NoError(t, err, "failed to get klio-plugin logs")

	require.Contains(t, logs, "Backup verification completed successfully",
		"backup verification log message not found in klio-plugin logs")
	t.Log("Verified that backup verification ran successfully")
}

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

	cnpgCluster := cnpg.GetCnpgClusterObject("test-cluster", namespace, instances, "klio-plugin-configuration",
		cnpg.ClusterTemplateOptions{ImagePullSecret: pullSecretName()})

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace, "klio-user@test-cluster", caIssuer)
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: userCertificate,
			ClusterName:       "test-cluster",
		},
	)
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets("encryption", namespace, "testencryptionpassword123")
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			Image:              testCfg.ServerImage,
			StorageClass:       testCfg.StorageClass,
			ImagePullSecret:    pullSecretName(),
			TLSSecretName:      certificate.Spec.SecretName,
			ClientCASecretName: caCertificate.Spec.SecretName,
			Encryption: klio.EncryptionOptions{
				EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
				EncryptionKeyFileName:   "encryption-key.age",
				IdentitySecretName:      ageSecrets.IdentitySecret.Name,
				IdentityFileName:        "identity.txt",
			},
		},
	)

	backup := cnpg.GetCnpgBackupObject("test-backup", namespace, backupTarget, cnpgCluster)

	c := commonBackupRestoreScenario{
		namespace:                     namespaceObj,
		cnpgCluster:                   cnpgCluster,
		userCertificate:               userCertificate,
		encryptionSecret:              ageSecrets.EncryptionKeySecret,
		identitySecret:                ageSecrets.IdentitySecret,
		issuer:                        issuer,
		caIssuer:                      caIssuer,
		caCertificate:                 caCertificate,
		certificate:                   certificate,
		klioServer:                    klioServer,
		klioPluginConfigurationSource: klioPluginConfiguration,
		name:                          name,
	}

	return machineryFeatures.NewBackupFeature(machineryFeatures.BackupFeatureConfig{
		Name:             name,
		Setup:            c.Setup,
		Teardown:         c.Teardown,
		Backup:           backup,
		PostBackupAssert: assertVerificationRan,
	})
}

func BackupFromPrimary(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromPrimary", cnpgv1.BackupTargetPrimary, 1, namespace)
}

func BackupFromStandby(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromStandby", cnpgv1.BackupTargetStandby, 2, namespace)
}

package e2e

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
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

	cnpgCluster := cnpg.GetCnpgClusterObject("test-cluster", namespace, instances, "klio-plugin-configuration")

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace, "klio-user@test-cluster", caIssuer)
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: userCertificate,
		},
	)
	encryptionSecret := secrets.GetKlioEncryptionSecret("encryption", namespace, "testencryptionpassword123")
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			TLSSecretName:        certificate.Spec.SecretName,
			ClientCASecretName:   caCertificate.Spec.SecretName,
			EncryptionSecretName: encryptionSecret.Name,
		},
	)

	backup := cnpg.GetCnpgBackupObject("test-backup", namespace, backupTarget, cnpgCluster)

	c := commonBackupRestoreScenario{
		namespace:                     namespaceObj,
		cnpgCluster:                   cnpgCluster,
		userCertificate:               userCertificate,
		encryptionSecret:              encryptionSecret,
		issuer:                        issuer,
		caIssuer:                      caIssuer,
		caCertificate:                 caCertificate,
		certificate:                   certificate,
		klioServer:                    klioServer,
		klioPluginConfigurationSource: klioPluginConfiguration,
		name:                          name,
	}

	return machineryFeatures.NewBackupFeature(machineryFeatures.BackupFeatureConfig{
		Name:     name,
		Setup:    c.Setup,
		Teardown: c.Teardown,
		Backup:   backup,
	})
}

func BackupFromPrimary(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromPrimary", cnpgv1.BackupTargetPrimary, 1, namespace)
}

func BackupFromStandby(namespace string) *machineryFeatures.BackupFeature {
	return newBackupFeature("BackupFromStandby", cnpgv1.BackupTargetStandby, 2, namespace)
}

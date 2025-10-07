package e2e

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/secrets"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates"
)

func NewTablespaceRecoveryFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.TablespaceRecoveryFeatureConfig {
	const klioServerName = "test-klio-server"

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	tablespaceConfig := []cnpgv1.TablespaceConfiguration{
		{
			Name: "tbs1",
			Owner: cnpgv1.DatabaseRoleRef{
				Name: "postgres",
			},
			Storage: cnpgv1.StorageConfiguration{
				Size: "1G",
			},
		},
		{
			Name: "tbs2",
			Owner: cnpgv1.DatabaseRoleRef{
				Name: "app",
			},
			Storage: cnpgv1.StorageConfiguration{
				Size: "2G",
			},
		},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	clientSecret := secrets.GetKlioClientSecret("klio-client", namespace, "klio", "testclientpassword123")
	cnpgCluster := templates.GetCnpgClusterWithTablespacesObject("test-cluster-source", namespace, instances, certificate,
		clientSecret, tablespaceConfig)

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
				"backupRef":        backup.Name,
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

	c := commonBackupRestoreScenario{
		namespace:        namespaceObj,
		clientSecret:     clientSecret,
		cnpgCluster:      cnpgCluster,
		userSecret:       userSecret,
		encryptionSecret: encryptionSecret,
		issuer:           issuer,
		certificate:      certificate,
		klioServer:       klioServer,

		name: name,
	}

	return machineryFeatures.TablespaceRecoveryFeatureConfig{
		RecoveryFeatureConfig: &machineryFeatures.RecoveryFeatureConfig{
			Name:             name,
			Setup:            c.Setup,
			Teardown:         c.Teardown,
			SourcePrimaryPod: &c.sourcePrimaryPod,
			Backup:           backup,
			RecoveryCluster:  recoveryCluster,
		},
		SourceTablespaceConfig: &tablespaceConfig,
	}
}

func RecoverClusterWithTablespaces(namespace string) *machineryFeatures.TablespaceRecoveryFeature {
	return machineryFeatures.NewTablespaceRecoveryFeature(NewTablespaceRecoveryFeatureConfig(
		"RecoverClusterWithTablespaces", 1, namespace))
}

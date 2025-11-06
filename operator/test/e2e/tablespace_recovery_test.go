package e2e

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/mutators"
	"github.com/cloudnative-pg/klio/operator/test/utils/secrets"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates"
)

func NewTablespaceRecoveryFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.TablespaceRecoveryFeatureConfig {
	const (
		externalClusterName     = "source-cluster"
		cnpgSourceClusterName   = "test-cluster-source"
		cnpgRestoredClusterName = "test-cluster-restore"
	)
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

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	cnpgCluster := templates.GetCnpgClusterWithTablespacesObject(cnpgSourceClusterName, namespace, instances,
		"klio-plugin-configuration", tablespaceConfig)

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace,
		"klio-user@"+cnpgSourceClusterName, caIssuer)
	klioPluginConfigurationSource := templates.GetKlioPluginConfigurationObject(
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

	backup := templates.GetCnpgBackupObject("test-backup", namespace, cnpgv1.DefaultBackupTarget, cnpgCluster)

	// Generate the recovery Cluster object
	recoveryCluster := cnpgCluster.DeepCopy()
	recoveryCluster.Name = cnpgRestoredClusterName
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: externalClusterName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: ptr.To(true),
			Parameters: map[string]string{
				cnpgi.PluginConfigurationRefParam: "klio-plugin-configuration-recovery",
			},
		},
	}}
	recoveryCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: externalClusterName,
		},
	}
	// TODO: we should check that WAL archiving and backups also work in the recovered Cluster
	recoveryCluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	// Generate the Klio PluginConfiguration for recovery
	klioPluginConfigurationRecovery := klioPluginConfigurationSource.DeepCopy()
	klioPluginConfigurationRecovery.Name = "klio-plugin-configuration-recovery"
	klioPluginConfigurationRecovery.Spec.ClusterName = cnpgCluster.Name

	c := commonBackupRestoreScenario{
		namespace:                       namespaceObj,
		cnpgCluster:                     cnpgCluster,
		userCertificate:                 userCertificate,
		encryptionSecret:                encryptionSecret,
		issuer:                          issuer,
		certificate:                     certificate,
		caCertificate:                   caCertificate,
		caIssuer:                        caIssuer,
		klioServer:                      klioServer,
		klioPluginConfigurationSource:   klioPluginConfigurationSource,
		klioPluginConfigurationRecovery: klioPluginConfigurationRecovery,
		name:                            name,
	}

	mutatorFuncs := []machineryFeatures.RecoveryClusterMutateFunc{
		mutators.CreateBackupIDMutator(backup, klioPluginConfigurationRecovery.Name,
			klioPluginConfigurationRecovery.Namespace),
	}

	return machineryFeatures.TablespaceRecoveryFeatureConfig{
		RecoveryFeatureConfig: &machineryFeatures.RecoveryFeatureConfig{
			Name:                  name,
			Setup:                 c.Setup,
			Teardown:              c.Teardown,
			SourcePrimaryPod:      &c.sourcePrimaryPod,
			Backup:                backup,
			RecoveryCluster:       recoveryCluster,
			MutateRecoveryCluster: mutatorFuncs,
		},
		SourceTablespaceConfig: &tablespaceConfig,
	}
}

func RecoverClusterWithTablespaces(namespace string) *machineryFeatures.TablespaceRecoveryFeature {
	return machineryFeatures.NewTablespaceRecoveryFeature(NewTablespaceRecoveryFeatureConfig(
		"RecoverClusterWithTablespaces", 1, namespace))
}

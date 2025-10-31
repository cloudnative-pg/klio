package e2e

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/secrets"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates"
)

type recoveryStrategy string

const (
	recoveryFromBackupRef recoveryStrategy = "fromBackupRef"
	recoveryFromBackupID  recoveryStrategy = "fromBackupID"
)

func NewRecoveryFeatureConfig(
	name string, instances int, namespace string, strategy recoveryStrategy,
) machineryFeatures.RecoveryFeatureConfig {
	const klioServerName = "test-klio-server"
	var mutators []machineryFeatures.RecoveryClusterMutateFunc

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	cnpgCluster := templates.GetCnpgClusterObject("test-cluster-source", namespace, instances,
		"klio-plugin-configuration")

	userCertificate := certificates.GetUserCertificateObject(
		"klio-user",
		namespace,
		"klio-user@test-cluster-source",
		caIssuer,
	)
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
	recoveryCluster.Name = "test-cluster-restored"
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: "source-cluster",
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
			Source: "source-cluster",
		},
	}
	// TODO: we should check that WAL archiving and backups also work in the recovered Cluster
	recoveryCluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	// Generate the Klio PluginConfiguration for recovery
	klioPluginConfigurationRecovery := klioPluginConfigurationSource.DeepCopy()
	klioPluginConfigurationRecovery.Name = "klio-plugin-configuration-recovery"
	klioPluginConfigurationRecovery.Spec.BackupRef = backup.Name
	klioPluginConfigurationRecovery.Spec.ClusterName = cnpgCluster.Name

	switch strategy {
	case recoveryFromBackupRef:
		klioPluginConfigurationRecovery.Spec.BackupRef = backup.Name
	case recoveryFromBackupID:
		mutators = append(mutators, func(cluster *cnpgv1.Cluster, rc machineryFeatures.RecoveryContext) {
			klioPluginConfigurationRecovery.Spec.BackupID = rc.BackupID
			cluster.Spec.Bootstrap.Recovery.RecoveryTarget = &cnpgv1.RecoveryTarget{
				TargetImmediate: ptr.To(true),
				BackupID:        rc.BackupID,
			}
		})
	}

	c := commonBackupRestoreScenario{
		namespace:                       namespaceObj,
		cnpgCluster:                     cnpgCluster,
		userCertificate:                 userCertificate,
		encryptionSecret:                encryptionSecret,
		issuer:                          issuer,
		caIssuer:                        caIssuer,
		caCertificate:                   caCertificate,
		certificate:                     certificate,
		klioServer:                      klioServer,
		klioPluginConfigurationSource:   klioPluginConfigurationSource,
		klioPluginConfigurationRecovery: klioPluginConfigurationRecovery,
		name:                            name,
	}

	recoveryConfig := machineryFeatures.RecoveryFeatureConfig{
		Name:             name,
		Setup:            c.Setup,
		Teardown:         c.Teardown,
		SourcePrimaryPod: &c.sourcePrimaryPod,
		Backup:           backup,
		RecoveryCluster:  recoveryCluster,
	}
	if len(mutators) > 0 {
		recoveryConfig.MutateRecoveryCluster = mutators
	}

	return recoveryConfig
}

func RecoverClusterFromBackupRef(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromBackupRef", 1, namespace, recoveryFromBackupRef))
}

func RecoverClusterFromBackupID(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromBackupID", 1, namespace, recoveryFromBackupID))
}

func RecoverClusterFromPitr(namespace string) *machineryFeatures.PitrFeature {
	return machineryFeatures.NewPitrFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromPitr", 1, namespace, recoveryFromBackupRef))
}

package e2e

import (
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/mutators"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

func NewRecoveryFeatureConfig(
	name string, instances int, namespace string, useBackupID bool,
) machineryFeatures.RecoveryFeatureConfig {
	const (
		externalClusterName     = "source-cluster"
		cnpgSourceClusterName   = "test-cluster-source"
		cnpgRestoredClusterName = "test-cluster-restore"
	)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	cnpgCluster := cnpg.GetCnpgClusterObject(cnpgSourceClusterName, namespace, instances,
		"klio-plugin-configuration",
		cnpg.ClusterTemplateOptions{ImagePullSecret: pullSecretName()})

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace,
		"klio-user@"+cnpgSourceClusterName, caIssuer)
	klioPluginConfigurationSource := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: userCertificate,
			ClusterName:       cnpgSourceClusterName,
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

	backup := cnpg.GetCnpgBackupObject("test-backup", namespace, cnpgv1.DefaultBackupTarget, cnpgCluster)

	// Generate the recovery Cluster object
	recoveryCluster := cnpgCluster.DeepCopy()
	recoveryCluster.Name = cnpgRestoredClusterName
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: externalClusterName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: new(true),
			Parameters: map[string]string{
				klioconfig.PluginConfigurationRefParam: "klio-plugin-configuration-recovery",
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
		encryptionSecret:                ageSecrets.EncryptionKeySecret,
		identitySecret:                  ageSecrets.IdentitySecret,
		issuer:                          issuer,
		caIssuer:                        caIssuer,
		caCertificate:                   caCertificate,
		certificate:                     certificate,
		klioServer:                      klioServer,
		klioPluginConfigurationSource:   klioPluginConfigurationSource,
		klioPluginConfigurationRecovery: klioPluginConfigurationRecovery,
		name:                            name,
	}

	var mutatorsFuncs []machineryFeatures.RecoveryClusterMutateFunc
	if useBackupID {
		mutatorsFuncs = append(mutatorsFuncs, mutators.CreateBackupIDMutator(backup))
	}

	recoveryConfig := machineryFeatures.RecoveryFeatureConfig{
		Name:                  name,
		Setup:                 c.Setup,
		Teardown:              c.Teardown,
		SourcePrimaryPod:      &c.sourcePrimaryPod,
		Backup:                backup,
		RecoveryCluster:       recoveryCluster,
		MutateRecoveryCluster: mutatorsFuncs,
	}

	return recoveryConfig
}

func RecoverClusterFromBackupID(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromBackupID", 1, namespace, true))
}

func RecoverClusterFromLatestBackup(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromLatestBackup", 1, namespace, false))
}

func RecoverClusterFromPitr(namespace string) *machineryFeatures.PitrFeature {
	return machineryFeatures.NewPitrFeature(NewRecoveryFeatureConfig(
		"RecoverClusterFromPitr", 1, namespace, false))
}

func RecoverReplicaCluster(namespace string) *machineryFeatures.ReplicaClusterFeature {
	return machineryFeatures.NewReplicaClusterFeature(NewRecoveryFeatureConfig(
		"RecoverReplicaCluster", 1, namespace, false))
}

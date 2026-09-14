/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package e2e

import (
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// Tier1Retention returns a Tier1RetentionFeature for a tier1-only deployment.
// It keeps the two most recent backups, takes three, and verifies the retention
// manager deleted the oldest, then tightens the policy to one and verifies
// `klio retention apply` leaves only the newest. This exercises the tier1
// retention path, which is distinct from the tier2 one covered by
// Tier2Retention.
func Tier1Retention(namespace string) *klioFeatures.Tier1RetentionFeature {
	const (
		cnpgClusterName         = "pg-tier1-retention"
		pluginConfigurationName = "klio-plugin-configuration"
		keepLatest              = 2
	)

	namespaceObj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)
	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)
	userCertificate := certificates.GetUserCertificateObject(
		"klio-user", namespace, "klio-user@"+cnpgClusterName, caIssuer)

	cnpgCluster := cnpg.GetCnpgClusterObject(cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})

	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:    certificate,
			ClientCertificate:    userCertificate,
			ClusterName:          cnpgClusterName,
			Tier1RetentionPolicy: &kliov1alpha1.RetentionPolicy{Latest: keepLatest},
		},
	)

	ageSecrets := secrets.GetKlioAgeEncryptionSecrets("encryption", namespace, "testencryptionpassword123")
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			Image:              testCfg.ServerImage,
			StorageClass:       testCfg.StorageClass,
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

	// Take one more backup than the policy keeps, so retention must delete one.
	backups := make([]*cnpgv1.Backup, 0, keepLatest+1)
	for i := range keepLatest + 1 {
		backups = append(backups, cnpg.GetCnpgBackupObject(
			fmt.Sprintf("test-backup-%d", i+1), namespace, cnpgv1.BackupTargetPrimary, cnpgCluster))
	}

	scenario := commonBackupRestoreScenario{
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
		name:                          "Tier1Retention",
	}

	return klioFeatures.NewTier1RetentionFeature(klioFeatures.Tier1RetentionFeatureConfig{
		Name:                    "Tier1Retention",
		Setup:                   scenario.Setup,
		Teardown:                scenario.Teardown,
		Backups:                 backups,
		KlioServer:              klioServer,
		Namespace:               namespace,
		KeepLatest:              keepLatest,
		ClusterName:             cnpgClusterName,
		PluginConfigurationName: pluginConfigurationName,
	})
}

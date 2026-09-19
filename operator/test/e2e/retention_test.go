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
	"context"
	"fmt"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// retentionScenario contains all resources needed for combined
// tier1/tier2 retention testing.
type retentionScenario struct {
	// Common
	namespace *corev1.Namespace
	issuer    *certmanagerv1.Issuer

	// RustFS infrastructure
	rustfsSecret          *corev1.Secret
	rustfsConfigMap       *corev1.ConfigMap
	rustfsCertificate     *certmanagerv1.Certificate
	rustfsService         *corev1.Service
	rustfsDeployment      *appsv1.Deployment
	rustfsCreateBucketJob *batchv1.Job

	// Klio Server with tier2
	serverCertificate *certmanagerv1.Certificate
	caCertificate     *certmanagerv1.Certificate
	caIssuer          *certmanagerv1.Issuer
	userCertificate   *certmanagerv1.Certificate
	encryptionSecret  *corev1.Secret
	identitySecret    *corev1.Secret
	klioServer        *kliov1alpha1.Server

	// Source cluster
	cnpgCluster      *cnpgv1.Cluster
	klioPluginConfig *kliov1alpha1.PluginConfiguration
	backups          []*cnpgv1.Backup
	name             string
}

// Setup creates all resources for the retention test.
func (s *retentionScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	createNamespace(ctx, t, r, s.namespace)

	scenario := infra.Tier2{
		Issuer:                s.issuer,
		RustfsSecret:          s.rustfsSecret,
		RustfsConfigMap:       s.rustfsConfigMap,
		RustfsCertificate:     s.rustfsCertificate,
		RustfsService:         s.rustfsService,
		RustfsDeployment:      s.rustfsDeployment,
		RustfsCreateBucketJob: s.rustfsCreateBucketJob,
		ServerCertificate:     s.serverCertificate,
		CaCertificate:         s.caCertificate,
		CaIssuer:              s.caIssuer,
		UserCertificate:       s.userCertificate,
		EncryptionSecret:      s.encryptionSecret,
		IdentitySecret:        s.identitySecret,
		KlioServer:            s.klioServer,
	}

	scenario.ParallelSetup(ctx, t, r)

	t.Logf("Deploying CNPG cluster...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfig),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG cluster")

	t.Logf("Waiting for cluster to be ready...")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "cluster not ready")

	t.Logf("All resources created and ready for retention feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *retentionScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for retention feature: %s", s.name)

	return ctx
}

// newRetentionFeatureConfig creates a new retention feature configuration.
// It configures a single-instance cluster with tier1 latest=1 and tier2
// latest=2, and takes one more backup than the larger of the two so both
// tiers' retention is exercised.
func newRetentionFeatureConfig(name, namespace string, tier1Keep, tier2Keep int32) klioFeatures.RetentionFeatureConfig {
	const (
		cnpgClusterName = "pg-retention"
		klioServerName  = "klio"

		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		clientCertName        = cnpgClusterName + "-client"

		rustfsName                = "rustfs"
		rustfsSecretName          = rustfsName + "-secret"
		rustfsConfigMapName       = rustfsName + "-config"
		rustfsCreateBucketJobName = rustfsName

		encryptionSecretName = "encryption"
		encryptionPassword   = "testencryptionpassword123"

		pluginConfigurationName = "klio-plugin-configuration"

		s3Prefix = "tier2"
	)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject(selfSignedIssuerName, namespace)

	rustfsSecret := rustfs.GetRustFSSecret(rustfsSecretName, namespace)
	rustfsConfigMap := rustfs.GetRustFSConfigMap(rustfsConfigMapName, namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(rustfsName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(rustfsName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(rustfsName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		rustfsCreateBucketJobName, namespace, rustfs.RustFSBucketName)

	caCertificate := certificates.GetCACertificateObject(caCertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(caIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(serverCertificateName, namespace, []string{klioServerName},
		issuer)
	userCertificate := certificates.GetUserCertificateObject(
		clientCertName, namespace, clientCertName+"@"+cnpgClusterName, caIssuer)

	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(encryptionSecretName, namespace, encryptionPassword)

	klioServer := klio.GetServerWithTier2Object(
		klioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				Image:              testCfg.ServerImage,
				StorageClass:       testCfg.StorageClass,
				TLSSecretName:      serverCertificate.Spec.SecretName,
				ClientCASecretName: caCertificate.Spec.SecretName,
				Encryption: klio.EncryptionOptions{
					EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
					EncryptionKeyFileName:   "encryption-key.age",
					IdentitySecretName:      ageSecrets.IdentitySecret.Name,
					IdentityFileName:        "identity.txt",
				},
			},
			Tier2Encryption: klio.EncryptionOptions{
				EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
				EncryptionKeyFileName:   "encryption-key.age",
				IdentitySecretName:      ageSecrets.IdentitySecret.Name,
				IdentityFileName:        "identity.txt",
			},
			S3: klio.Tier2S3Options{
				S3BucketName:          rustfs.RustFSBucketName,
				S3Prefix:              s3Prefix,
				S3Endpoint:            rustfs.GetRustFSEndpoint(rustfsName, namespace),
				S3Region:              rustfs.RustFSRegion,
				S3AccessKeySecretName: rustfsSecret.Name,
				S3SecretKeySecretName: rustfsSecret.Name,
				S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
			},
		},
	)

	// Single-instance cluster: tier1 keeps latest=1, so a single instance
	// keeps the WAL-floor assertion unambiguous (no standby archiving of
	// its own).
	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})

	klioPluginConfig := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			ClusterName:         cnpgClusterName,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			Mode:                kliov1alpha1.ModeStandard,
			Tier2RetentionPolicy: &kliov1alpha1.RetentionPolicy{
				Latest: new(tier2Keep),
			},
		},
	)
	klioPluginConfig.Spec.Tier1 = &kliov1alpha1.Tier1PluginConfiguration{
		RetentionPolicy: &kliov1alpha1.RetentionPolicy{
			Latest: new(tier1Keep),
		},
	}

	// Take one more backup than the larger of the two retention values, so
	// both tiers are forced to prune at least once.
	backupCount := max(tier1Keep, tier2Keep) + 1
	backups := make([]*cnpgv1.Backup, 0, backupCount)
	for i := range backupCount {
		backupName := fmt.Sprintf("test-backup-%d", i+1)
		backups = append(backups, cnpg.GetCnpgBackupObject(backupName, namespace, cnpgv1.DefaultBackupTarget, cnpgCluster))
	}

	scenario := &retentionScenario{
		namespace:             namespaceObj,
		issuer:                issuer,
		rustfsSecret:          rustfsSecret,
		rustfsConfigMap:       rustfsConfigMap,
		rustfsCertificate:     rustfsCertificate,
		rustfsService:         rustfsService,
		rustfsDeployment:      rustfsDeployment,
		rustfsCreateBucketJob: rustfsCreateBucketJob,
		serverCertificate:     serverCertificate,
		caCertificate:         caCertificate,
		caIssuer:              caIssuer,
		userCertificate:       userCertificate,
		encryptionSecret:      ageSecrets.EncryptionKeySecret,
		identitySecret:        ageSecrets.IdentitySecret,
		klioServer:            klioServer,
		cnpgCluster:           cnpgCluster,
		klioPluginConfig:      klioPluginConfig,
		backups:               backups,
		name:                  name,
	}

	return klioFeatures.RetentionFeatureConfig{
		Name:         name,
		Setup:        scenario.Setup,
		Teardown:     scenario.Teardown,
		Backups:      backups,
		KlioServer:   klioServer,
		Namespace:    namespace,
		Tier1Keep:    tier1Keep,
		Tier2Keep:    tier2Keep,
		ClusterName:  cnpgClusterName,
		S3BucketName: rustfs.RustFSBucketName,
		S3Prefix:     s3Prefix,
		RustFSName:   rustfsName,
	}
}

// Retention returns a RetentionFeature verifying that tier1 and tier2
// backup and WAL retention converge together: tier1 keeps only its latest
// backup, tier2 keeps its latest two, and each tier's oldest remaining WAL
// segment matches what its surviving backups require.
func Retention(namespace string) *klioFeatures.RetentionFeature {
	return klioFeatures.NewRetentionFeature(
		newRetentionFeatureConfig("Retention", namespace, 1, 2))
}

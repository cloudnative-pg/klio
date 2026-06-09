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
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// tier2RetentionScenario contains all resources needed for tier2 retention testing.
// This tests both backup retention (Kopia snapshots) and WAL retention.
type tier2RetentionScenario struct {
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
	cnpgCluster           *cnpgv1.Cluster
	klioPluginConfig      *kliov1alpha1.PluginConfiguration
	backups               []*cnpgv1.Backup
	name                  string
	tier2RetentionKeepNum int
}

// Setup creates all resources for the tier2 retention test.
func (s *tier2RetentionScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for tier2 retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create namespace
	createNamespace(ctx, t, r, s.namespace)

	// Set scenario infra
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

	// Parallel setup of RustFS and Klio Server for Tier2 scenario
	scenario.ParallelSetup(ctx, t, r)

	// Deploy CNPG cluster
	t.Logf("Deploying CNPG cluster...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfig),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG cluster")

	// Wait for cluster to be ready
	t.Logf("Waiting for cluster to be ready...")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "cluster not ready")

	t.Logf("All resources created and ready for tier2 retention feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *tier2RetentionScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for tier2 retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for tier2 retention feature: %s", s.name)

	return ctx
}

// NewTier2RetentionFeatureConfig creates a new tier2 retention feature configuration.
// This configures a test that validates both backup retention and WAL retention in tier2.
func NewTier2RetentionFeatureConfig(
	name string, namespace string, tier2RetentionKeepNum int,
) klioFeatures.Tier2RetentionFeatureConfig {
	const (
		// Cluster names
		cnpgClusterName = "pg-retention"

		// Server names
		klioServerName = "klio"

		// Certificate and issuer names
		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		clientCertName        = cnpgClusterName + "-client"

		// RustFS resource names
		rustfsName                = "rustfs"
		rustfsSecretName          = rustfsName + "-secret"
		rustfsConfigMapName       = rustfsName + "-config"
		rustfsCreateBucketJobName = rustfsName

		// Secret names
		encryptionSecretName = "encryption"
		encryptionPassword   = "testencryptionpassword123"

		// Plugin configuration names
		pluginConfigurationName = "klio-plugin-configuration"

		// S3 configuration
		s3Prefix = "tier2"
	)

	// Namespace
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	// Issuer for all certificates
	issuer := certificates.GetSelfSignedIssuerObject(selfSignedIssuerName, namespace)

	// RustFS infrastructure
	rustfsSecret := rustfs.GetRustFSSecret(rustfsSecretName, namespace)
	rustfsConfigMap := rustfs.GetRustFSConfigMap(rustfsConfigMapName, namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(rustfsName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(rustfsName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(rustfsName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		rustfsCreateBucketJobName, namespace, rustfs.RustFSBucketName)

	// Klio Server certificates and secrets
	caCertificate := certificates.GetCACertificateObject(caCertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(caIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(serverCertificateName, namespace, []string{klioServerName},
		issuer)
	userCertificate := certificates.GetUserCertificateObject(
		clientCertName, namespace, clientCertName+"@"+cnpgClusterName, caIssuer)

	// Encryption secret
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(encryptionSecretName, namespace, encryptionPassword)

	// Klio Server with tier2
	klioServer := klio.GetServerWithTier2Object(
		klioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				Image:              testCfg.ServerImage,
				StorageClass:       testCfg.StorageClass,
				ImagePullSecret:    pullSecretName(),
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

	// CNPG cluster
	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{ImagePullSecret: pullSecretName(), StorageClass: testCfg.StorageClass})

	// Plugin configuration with tier2 backup enabled and retention policy
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
				KeepLatest:  new(tier2RetentionKeepNum),
				KeepHourly:  new(0),
				KeepDaily:   new(0),
				KeepWeekly:  new(0),
				KeepMonthly: new(0),
				KeepAnnual:  new(0),
			},
		},
	)

	// Create multiple backups for retention testing
	backups := make([]*cnpgv1.Backup, 0, tier2RetentionKeepNum+1)
	for i := range tier2RetentionKeepNum + 1 {
		backupName := fmt.Sprintf("test-backup-%d", i+1)
		backups = append(backups, cnpg.GetCnpgBackupObject(backupName, namespace, cnpgv1.DefaultBackupTarget, cnpgCluster))
	}

	scenario := &tier2RetentionScenario{
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
		tier2RetentionKeepNum: tier2RetentionKeepNum,
	}

	return klioFeatures.Tier2RetentionFeatureConfig{
		Name:        name,
		Setup:       scenario.Setup,
		Teardown:    scenario.Teardown,
		Backups:     backups,
		KlioServer:  klioServer,
		Namespace:   namespace,
		KeepLatest:  tier2RetentionKeepNum,
		ClusterName: cnpgClusterName,
		S3Prefix:    s3Prefix,
	}
}

// Tier2Retention returns a Tier2RetentionFeature for testing tier2 retention.
// This test validates both backup retention (Kopia snapshots kept to keepLatest=1)
// and WAL retention (cleanup of WALs older than the oldest remaining backup).
func Tier2Retention(namespace string) *klioFeatures.Tier2RetentionFeature {
	return klioFeatures.NewTier2RetentionFeature(
		NewTier2RetentionFeatureConfig("Tier2Retention", namespace, 1))
}

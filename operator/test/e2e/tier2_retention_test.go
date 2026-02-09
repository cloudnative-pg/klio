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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	klioConditions "github.com/cloudnative-pg/klio/operator/test/utils/conditions"
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
	rustfsPVC             *corev1.PersistentVolumeClaim
	rustfsLogsPVC         *corev1.PersistentVolumeClaim
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
	require.NoError(t, r.Create(ctx, s.namespace), "failed to create namespace")

	// Deploy RustFS infrastructure
	t.Logf("Deploying RustFS infrastructure...")
	require.NoError(t, r.Create(ctx, s.rustfsSecret), "failed to create RustFS secret")
	require.NoError(t, r.Create(ctx, s.rustfsConfigMap), "failed to create RustFS configmap")
	require.NoError(t, r.Create(ctx, s.rustfsPVC), "failed to create RustFS data PVC")
	require.NoError(t, r.Create(ctx, s.rustfsLogsPVC), "failed to create RustFS logs PVC")
	require.NoError(t, r.Create(ctx, s.issuer), "failed to create issuer")

	// Wait for issuer to be ready before creating certificates
	err = wait.For(
		klioConditions.IssuerIsReady(r, s.issuer),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "issuer not ready")

	require.NoError(t, r.Create(ctx, s.rustfsCertificate), "failed to create RustFS certificate")

	// Wait for RustFS certificate to be ready
	err = wait.For(
		klioConditions.CertificateIsReady(r, s.rustfsCertificate),
		wait.WithTimeout(5*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "RustFS certificate not ready")

	require.NoError(t, r.Create(ctx, s.rustfsService), "failed to create RustFS service")
	require.NoError(t, r.Create(ctx, s.rustfsDeployment), "failed to create RustFS deployment")

	// Wait for RustFS deployment to be ready
	t.Logf("Waiting for RustFS deployment to be ready...")
	err = wait.For(
		klioConditions.DeploymentIsReady(r, s.rustfsDeployment),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "RustFS deployment not ready")

	// Create bucket
	t.Logf("Creating S3 bucket in RustFS...")
	require.NoError(t, r.Create(ctx, s.rustfsCreateBucketJob), "failed to create bucket creation job")

	// Wait for bucket creation to complete
	err = wait.For(
		klioConditions.JobIsComplete(r, s.rustfsCreateBucketJob),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "bucket creation job not complete")

	// Deploy Klio Server with tier2
	t.Logf("Deploying Klio Server with tier2...")
	require.NoError(t, r.Create(ctx, s.caCertificate), "failed to create CA certificate")
	require.NoError(t, r.Create(ctx, s.caIssuer), "failed to create CA issuer")

	// Wait for CA certificate to be ready
	err = wait.For(
		klioConditions.CertificateIsReady(r, s.caCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "CA certificate not ready")

	require.NoError(t, r.Create(ctx, s.serverCertificate), "failed to create server certificate")
	require.NoError(t, r.Create(ctx, s.userCertificate), "failed to create user certificate")

	// Wait for certificates to be ready
	err = wait.For(
		klioConditions.CertificateIsReady(r, s.serverCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "server certificate not ready")

	require.NoError(t, r.Create(ctx, s.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, s.klioServer), "failed to create Klio server")

	// Wait for Klio Server to be ready
	t.Logf("Waiting for Klio Server to be ready...")
	err = wait.For(
		klioConditions.KlioServerIsReady(r, s.klioServer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "Klio server not ready")

	// Deploy CNPG cluster
	t.Logf("Deploying CNPG cluster...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfig),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG cluster")

	// Wait for cluster to be ready
	t.Logf("Waiting for cluster to be ready...")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(2*time.Minute),
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
) machineryFeatures.Tier2RetentionFeatureConfig {
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
		rustfsDataPVCName         = rustfsName + "-data"
		rustfsLogsPVCName         = rustfsName + "-logs"
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
	rustfsPVC := rustfs.GetRustFSPVC(rustfsDataPVCName, namespace)
	rustfsLogsPVC := rustfs.GetRustFSLogsPVC(rustfsLogsPVCName, namespace)
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
	encryptionSecret := secrets.GetKlioEncryptionSecret(encryptionSecretName, namespace, encryptionPassword)

	// Klio Server with tier2
	klioServer := klio.GetServerWithTier2Object(
		klioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				TLSSecretName:        serverCertificate.Spec.SecretName,
				ClientCASecretName:   caCertificate.Spec.SecretName,
				EncryptionSecretName: encryptionSecret.Name,
			},
			Tier2EncryptionSecretName: encryptionSecret.Name,
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
		cnpgClusterName, namespace, 1, pluginConfigurationName)

	// Plugin configuration with tier2 backup enabled and retention policy
	klioPluginConfig := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			Mode:                kliov1alpha1.ModeStandard,
			Tier2RetentionPolicy: &kliov1alpha1.RetentionPolicy{
				KeepLatest:  ptr.To(tier2RetentionKeepNum),
				KeepHourly:  ptr.To(0),
				KeepDaily:   ptr.To(0),
				KeepWeekly:  ptr.To(0),
				KeepMonthly: ptr.To(0),
				KeepAnnual:  ptr.To(0),
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
		rustfsPVC:             rustfsPVC,
		rustfsLogsPVC:         rustfsLogsPVC,
		rustfsCertificate:     rustfsCertificate,
		rustfsService:         rustfsService,
		rustfsDeployment:      rustfsDeployment,
		rustfsCreateBucketJob: rustfsCreateBucketJob,
		serverCertificate:     serverCertificate,
		caCertificate:         caCertificate,
		caIssuer:              caIssuer,
		userCertificate:       userCertificate,
		encryptionSecret:      encryptionSecret,
		klioServer:            klioServer,
		cnpgCluster:           cnpgCluster,
		klioPluginConfig:      klioPluginConfig,
		backups:               backups,
		name:                  name,
		tier2RetentionKeepNum: tier2RetentionKeepNum,
	}

	return machineryFeatures.Tier2RetentionFeatureConfig{
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
func Tier2Retention(namespace string) *machineryFeatures.Tier2RetentionFeature {
	return machineryFeatures.NewTier2RetentionFeature(
		NewTier2RetentionFeatureConfig("Tier2Retention", namespace, 1))
}

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// tier2RecoveryScenario contains all resources needed for tier2 recovery testing.
type tier2RecoveryScenario struct {
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
	cnpgCluster                   *cnpgv1.Cluster
	klioPluginConfigurationSource *kliov1alpha1.PluginConfiguration

	// Recovery resources (created in mutator, not in Setup)
	recoveryServerCertificate       *certmanagerv1.Certificate
	recoveryServerCACertificate     *certmanagerv1.Certificate
	recoveryServerCAIssuer          *certmanagerv1.Issuer
	recoveryUserCertificate         *certmanagerv1.Certificate
	recoveryServer                  *kliov1alpha1.Server
	klioPluginConfigurationRecovery *kliov1alpha1.PluginConfiguration

	name             string
	sourcePrimaryPod corev1.Pod
}

// Setup creates all resources except RecoverySource (which is created in the mutator).
func (s *tier2RecoveryScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for tier2 recovery feature: %s", s.name)
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
		conditions.IssuerIsReady(r, s.issuer),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "issuer not ready")

	require.NoError(t, r.Create(ctx, s.rustfsCertificate), "failed to create RustFS certificate")

	// Wait for RustFS certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, s.rustfsCertificate),
		wait.WithTimeout(5*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "RustFS certificate not ready")

	require.NoError(t, r.Create(ctx, s.rustfsService), "failed to create RustFS service")
	require.NoError(t, r.Create(ctx, s.rustfsDeployment), "failed to create RustFS deployment")

	// Wait for RustFS deployment to be ready
	t.Logf("Waiting for RustFS deployment to be ready...")
	err = wait.For(
		conditions.DeploymentIsReady(r, s.rustfsDeployment),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "RustFS deployment not ready")

	// Create bucket
	t.Logf("Creating S3 bucket in RustFS...")
	require.NoError(t, r.Create(ctx, s.rustfsCreateBucketJob), "failed to create bucket creation job")

	// Wait for bucket creation to complete
	err = wait.For(
		conditions.JobIsComplete(r, s.rustfsCreateBucketJob),
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
		conditions.CertificateIsReady(r, s.caCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "CA certificate not ready")

	require.NoError(t, r.Create(ctx, s.serverCertificate), "failed to create server certificate")
	require.NoError(t, r.Create(ctx, s.userCertificate), "failed to create user certificate")

	// Wait for certificates to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, s.serverCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "server certificate not ready")

	require.NoError(t, r.Create(ctx, s.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, s.klioServer), "failed to create Klio server")

	// Wait for Klio Server to be ready
	t.Logf("Waiting for Klio Server to be ready...")
	err = wait.For(
		conditions.KlioServerIsReady(r, s.klioServer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "Klio server not ready")

	// Deploy source CNPG cluster
	t.Logf("Deploying source CNPG cluster...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfigurationSource),
		"failed to create Klio plugin configuration for source cluster")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG source cluster")

	// Wait for source cluster to be ready
	t.Logf("Waiting for source cluster to be ready...")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "source cluster not ready")

	// Get source primary pod
	require.NoError(
		t,
		r.Get(
			ctx,
			s.cnpgCluster.Status.CurrentPrimary,
			s.namespace.Name,
			&s.sourcePrimaryPod,
		),
		"failed to get the current primary pod",
	)

	t.Logf("All resources created and ready for tier2 recovery feature: %s", s.name)
	t.Logf("NOTE: Second Klio Server will be created after backup reaches tier2 (in mutator phase)")

	return ctx
}

// Teardown deletes all resources.
func (s *tier2RecoveryScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for tier2 recovery feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for tier2 recovery feature: %s", s.name)

	return ctx
}

// checkTier2HasOneBackup checks if tier2 replication is complete by verifying that
// kopia snapshot list returns 3 snapshots in the tier2 storage.
func checkTier2HasOneBackup(
	r *resources.Resources,
	namespace string,
	serverName string,
) k8swait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podName := serverName + "-klio-0"
		containerName := "server"

		// Step 1: Find the tier2 config password file and derive config file path
		var stdout, stderr bytes.Buffer
		findCmd := []string{
			"sh", "-c",
			"ls /tmp/kopiaconfig_tier2_*.kopia-password 2>/dev/null",
		}

		err := r.ExecInPod(ctx, namespace, podName, containerName, findCmd, &stdout, &stderr)
		if err != nil {
			return false, fmt.Errorf("could not find kopia tier2 config: %w", err)
		}

		passwordFile := strings.TrimSpace(stdout.String())
		if passwordFile == "" {
			return false, nil // Password file not found yet, keep waiting
		}

		// Derive the config file path by removing the .kopia-password suffix
		configFile := strings.TrimSuffix(passwordFile, ".kopia-password")

		// Step 2: Run kopia snapshot list with the found config file
		stdout.Reset()
		stderr.Reset()
		kopiaCmd := []string{
			"kopia", "snapshot", "list",
			"--disable-file-logging",
			"--config-file=" + configFile,
			"--all",
			"--json",
		}

		err = r.ExecInPod(ctx, namespace, podName, containerName, kopiaCmd, &stdout, &stderr)
		if err != nil {
			return false, fmt.Errorf("failed to run kopia snapshot list: %w; stderr: %s", err, stderr.String())
		}

		// Step 3: Parse JSON output and check if we have 3 snapshots
		var snapshots []any
		if err := json.Unmarshal(stdout.Bytes(), &snapshots); err != nil {
			return false, fmt.Errorf("failed to parse kopia snapshot list output: %w", err)
		}

		// We expect 3 snapshots to be replicated to tier2
		return len(snapshots) == 3, nil
	}
}

// deployRecoveryServer creates the second Klio Server after tier2 replication.
//
//nolint:cyclop
func (s *tier2RecoveryScenario) deployRecoveryServer(
	ctx context.Context,
	_ *cnpgv1.Cluster,
	r *resources.Resources,
) error {
	// Wait for tier2 replication to complete (3 snapshots in tier2 storage)
	err := wait.For(
		checkTier2HasOneBackup(r, s.namespace.Name, s.klioServer.Name),
		wait.WithTimeout(5*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("tier2 replication not complete: %w", err)
	}

	// Now create second Klio Server after backup has reached tier2

	// Create second server's CA certificate and CA issuer
	if err := r.Create(ctx, s.recoveryServerCACertificate); err != nil {
		return fmt.Errorf("failed to create recovery server CA certificate: %w", err)
	}

	// Wait for CA certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, s.recoveryServerCACertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server CA certificate not ready: %w", err)
	}

	// Create CA issuer for second server
	if err := r.Create(ctx, s.recoveryServerCAIssuer); err != nil {
		return fmt.Errorf("failed to create recovery server CA issuer: %w", err)
	}

	// Create second server's certificate and recovery user certificate
	if err := r.Create(ctx, s.recoveryServerCertificate); err != nil {
		return fmt.Errorf("failed to create recovery server certificate: %w", err)
	}
	if err := r.Create(ctx, s.recoveryUserCertificate); err != nil {
		return fmt.Errorf("failed to create recovery user certificate: %w", err)
	}

	// Wait for server certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, s.recoveryServerCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server certificate not ready: %w", err)
	}

	// Wait for user certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, s.recoveryUserCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery user certificate not ready: %w", err)
	}

	// Create second Klio Server (shares tier2 storage with first server)
	if err := r.Create(ctx, s.recoveryServer); err != nil {
		return fmt.Errorf("failed to create recovery server: %w", err)
	}

	// Wait for second Server to be ready
	err = wait.For(
		conditions.KlioServerIsReady(r, s.recoveryServer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server not ready: %w", err)
	}

	// Create PluginConfiguration for recovery (points to second server)
	if err := r.Create(ctx, s.klioPluginConfigurationRecovery); err != nil {
		return fmt.Errorf("failed to create recovery plugin configuration: %w", err)
	}

	return nil
}

// NewTier2RecoveryFeatureConfig creates a new tier2 recovery feature configuration.
func NewTier2RecoveryFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.RecoveryFeatureConfig {
	const (
		// Cluster names
		cnpgSourceClusterName   = "pg-source"
		cnpgRestoredClusterName = "pg-restore"

		// Server names
		klioServerName          = "klio"
		klioTier2OnlyServerName = "klio-tier2-only"

		// Certificate and issuer names
		selfSignedIssuerName       = "selfsigned-issuer"
		caCertificateName          = klioServerName + "-ca"
		caIssuerName               = caCertificateName + "-issuer"
		serverCertificateName      = klioServerName + "-server"
		recoveryServerCACertName   = klioTier2OnlyServerName + "-ca"
		recoveryServerCAIssuerName = recoveryServerCACertName + "-issuer"
		recoveryServerCertName     = klioTier2OnlyServerName + "-server"
		cnpgSourceClientCertName   = cnpgSourceClusterName + "-client"
		cnpgRestoredClientCertName = cnpgRestoredClusterName + "client"

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
		sourcePluginConfigurationName   = "klio-plugin-configuration-source"
		recoveryPluginConfigurationName = "klio-plugin-configuration-recovery"

		// Backup names
		backupName = "test-backup"

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
		cnpgSourceClientCertName, namespace, cnpgSourceClientCertName+"@"+cnpgSourceClusterName, caIssuer)

	// Encryption secret (MUST be same for tier1 and tier2)
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
			Tier2EncryptionSecretName: encryptionSecret.Name, // Same encryption key for tier1 and tier2
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

	// Source CNPG cluster
	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgSourceClusterName, namespace, instances, sourcePluginConfigurationName)

	// Plugin configuration for source cluster (with tier2 backup enabled)
	klioPluginConfigurationSource := klio.GetPluginConfigurationObject(
		sourcePluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			ReadOnly:            false,
		},
	)

	// Backup
	backup := cnpg.GetCnpgBackupObject(backupName, namespace, cnpgv1.DefaultBackupTarget, cnpgCluster)

	// Second Klio Server (to be created in mutator)
	recoveryServerCACertificate := certificates.GetCACertificateObject(recoveryServerCACertName, namespace, issuer)
	recoveryServerCAIssuer := certificates.GetCAIssuerObject(recoveryServerCAIssuerName, namespace,
		recoveryServerCACertificate.Spec.SecretName)
	recoveryServerCertificate := certificates.GetCertificateObject(
		recoveryServerCertName, namespace, []string{klioTier2OnlyServerName}, recoveryServerCAIssuer)
	recoveryUserCertificate := certificates.GetUserCertificateObject(
		cnpgRestoredClientCertName, namespace, cnpgSourceClientCertName+"@"+cnpgSourceClusterName,
		recoveryServerCAIssuer)

	recoveryServer := klio.GetReadOnlyTier2ServerObject(
		klioTier2OnlyServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				TLSSecretName:        recoveryServerCertificate.Spec.SecretName,
				ClientCASecretName:   recoveryServerCACertificate.Spec.SecretName,
				EncryptionSecretName: encryptionSecret.Name, // SAME as first server
			},
			Tier2EncryptionSecretName: encryptionSecret.Name, // SAME as first server
			S3: klio.Tier2S3Options{
				S3BucketName:          rustfs.RustFSBucketName,
				S3Prefix:              s3Prefix, // SAME as first server
				S3Endpoint:            rustfs.GetRustFSEndpoint(rustfsName, namespace),
				S3Region:              rustfs.RustFSRegion,
				S3AccessKeySecretName: rustfsSecret.Name,
				S3SecretKeySecretName: rustfsSecret.Name,
				S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
			},
		},
	)

	// Plugin configuration for recovery cluster (to be created in mutator, with tier2 recovery enabled)
	klioPluginConfigurationRecovery := klio.GetPluginConfigurationObject(
		recoveryPluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   recoveryServerCertificate,
			ClientCertificate:   recoveryUserCertificate,
			EnableTier2Backup:   false,
			EnableTier2Recovery: true,
			ReadOnly:            true,
		},
	)
	klioPluginConfigurationRecovery.Spec.ClusterName = cnpgCluster.Name

	// Recovery cluster configuration
	recoveryCluster := cnpgCluster.DeepCopy()
	recoveryCluster.Name = cnpgRestoredClusterName
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: cnpgSourceClusterName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: ptr.To(true),
			Parameters: map[string]string{
				cnpgi.PluginConfigurationRefParam: klioPluginConfigurationRecovery.Name,
			},
		},
	}}
	recoveryCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: cnpgSourceClusterName,
		},
	}
	recoveryCluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	// Create scenario
	scenario := &tier2RecoveryScenario{
		namespace:                       namespaceObj,
		issuer:                          issuer,
		rustfsSecret:                    rustfsSecret,
		rustfsConfigMap:                 rustfsConfigMap,
		rustfsPVC:                       rustfsPVC,
		rustfsLogsPVC:                   rustfsLogsPVC,
		rustfsCertificate:               rustfsCertificate,
		rustfsService:                   rustfsService,
		rustfsDeployment:                rustfsDeployment,
		rustfsCreateBucketJob:           rustfsCreateBucketJob,
		serverCertificate:               serverCertificate,
		caCertificate:                   caCertificate,
		caIssuer:                        caIssuer,
		userCertificate:                 userCertificate,
		encryptionSecret:                encryptionSecret,
		klioServer:                      klioServer,
		cnpgCluster:                     cnpgCluster,
		klioPluginConfigurationSource:   klioPluginConfigurationSource,
		recoveryServerCertificate:       recoveryServerCertificate,
		recoveryServerCACertificate:     recoveryServerCACertificate,
		recoveryServerCAIssuer:          recoveryServerCAIssuer,
		recoveryUserCertificate:         recoveryUserCertificate,
		recoveryServer:                  recoveryServer,
		klioPluginConfigurationRecovery: klioPluginConfigurationRecovery,
		name:                            name,
	}

	recoveryConfig := machineryFeatures.RecoveryFeatureConfig{
		Name:                  name,
		Setup:                 scenario.Setup,
		Teardown:              scenario.Teardown,
		SourcePrimaryPod:      &scenario.sourcePrimaryPod,
		Backup:                backup,
		RecoveryCluster:       recoveryCluster,
		MutateRecoveryCluster: []machineryFeatures.RecoveryClusterMutateFunc{scenario.deployRecoveryServer},
	}

	return recoveryConfig
}

// RecoverClusterFromTier2 returns a RecoveryFeature for tier2 recovery testing.
func RecoverClusterFromTier2(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(
		NewTier2RecoveryFeatureConfig("RecoverClusterFromTier2", 1, namespace))
}

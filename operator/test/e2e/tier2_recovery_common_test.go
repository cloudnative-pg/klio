package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

const (
	// kopiaContainerName is the name of the container running kopia in the Klio server pod.
	serverContainerName = "server"

	// Tier2 test cluster names.
	tier2SourceClusterName   = "pg-source"
	tier2RestoredClusterName = "pg-restore"

	// Tier2 test server names.
	tier2KlioServerName         = "klio"
	tier2KlioReadOnlyServerName = "klio-tier2-only"

	// Tier2 test certificate and issuer names.
	tier2SelfSignedIssuerName       = "selfsigned-issuer"
	tier2CACertificateName          = tier2KlioServerName + "-ca"
	tier2CAIssuerName               = tier2CACertificateName + "-issuer"
	tier2ServerCertificateName      = tier2KlioServerName + "-server"
	tier2RecoveryServerCACertName   = tier2KlioReadOnlyServerName + "-ca"
	tier2RecoveryServerCAIssuerName = tier2RecoveryServerCACertName + "-issuer"
	tier2RecoveryServerCertName     = tier2KlioReadOnlyServerName + "-server"
	tier2SourceClientCertName       = tier2SourceClusterName + "-client"
	tier2RestoredClientCertName     = tier2RestoredClusterName + "-client"

	// Tier2 test RustFS resource names.
	tier2RustfsResourceName        = "rustfs"
	tier2RustfsSecretName          = tier2RustfsResourceName + "-secret"
	tier2RustfsConfigMapName       = tier2RustfsResourceName + "-config"
	tier2RustfsDataPVCName         = tier2RustfsResourceName + "-data"
	tier2RustfsLogsPVCName         = tier2RustfsResourceName + "-logs"
	tier2RustfsCreateBucketJobName = tier2RustfsResourceName

	// Tier2 test secret names.
	tier2EncryptionSecretName = "encryption"
	tier2EncryptionPassword   = "testencryptionpassword123"

	// Tier2 test plugin configuration names.
	tier2SourcePluginConfigName   = "klio-plugin-configuration-source"
	tier2RecoveryPluginConfigName = "klio-plugin-configuration-recovery"

	// Tier2 test backup names.
	tier2BackupName = "test-backup"

	// Tier2 S3 configuration.
	tier2S3Prefix = "tier2"

	// tier2AnnotationName is the annotation key used to mark backups present in tier2.
	tier2AnnotationName = "klio.io/tier2"
	//
	// presentAnnotationValue is the value set when a backup is present in a tier.
	presentAnnotationValue = "present"
)

// checkTier2ReplicationComplete checks if tier2 replication is complete by verifying that
// kopia snapshot list returns 3 snapshots in the tier2 storage.
func checkTier2ReplicationComplete(
	r *resources.Resources,
	namespace string,
	serverName string,
) k8swait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		podName := serverName + "-klio-0"

		// Use the Klio admin API to list backups
		var stdout, stderr bytes.Buffer
		klioCmd := []string{
			"klio", "admin", "list-backups",
		}

		if err := r.ExecInPod(ctx, namespace, podName, serverContainerName, klioCmd, &stdout, &stderr); err != nil {
			// Return false without error to keep retrying on transient failures
			return false, nil //nolint:nilerr
		}

		// Parse JSON output as BackupList
		type BackupMetadata struct {
			Name        string            `json:"name"`
			ClusterName string            `json:"clusterName"`
			Annotations map[string]string `json:"annotations,omitempty"`
		}

		var backups []BackupMetadata
		if err := json.Unmarshal(stdout.Bytes(), &backups); err != nil {
			// Return false without error to keep retrying on transient failures
			return false, nil //nolint:nilerr
		}

		// Count backups present in tier2 (those with the tier2 annotation)
		tier2Count := 0
		for i := range backups {
			if backups[i].Annotations[tier2AnnotationName] == presentAnnotationValue {
				tier2Count++
			}
		}

		return tier2Count == 1, nil
	}
}

// tier2RecoveryServerResources holds resources needed for a tier2 recovery server.
type tier2RecoveryServerResources struct {
	RecoveryServerCertificate   *certmanagerv1.Certificate
	RecoveryServerCACertificate *certmanagerv1.Certificate
	RecoveryServerCAIssuer      *certmanagerv1.Issuer
	RecoveryUserCertificate     *certmanagerv1.Certificate
	RecoveryServer              *kliov1alpha1.Server
	PluginConfigurationRecovery *kliov1alpha1.PluginConfiguration
}

// deployTier2RecoveryServer creates the recovery Klio Server after tier2 replication.
//
//nolint:cyclop
func deployTier2RecoveryServer(
	ctx context.Context,
	r *resources.Resources,
	namespace string,
	sourceServerName string,
	resources *tier2RecoveryServerResources,
) error {
	// Wait for tier2 replication to complete (3 snapshots in tier2 storage)
	err := wait.For(
		checkTier2ReplicationComplete(r, namespace, sourceServerName),
		wait.WithTimeout(5*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("tier2 replication not complete: %w", err)
	}

	// Now create second Klio Server after backup has reached tier2

	// Create second server's CA certificate and CA issuer
	if err := r.Create(ctx, resources.RecoveryServerCACertificate); err != nil {
		return fmt.Errorf("failed to create recovery server CA certificate: %w", err)
	}

	// Wait for CA certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, resources.RecoveryServerCACertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server CA certificate not ready: %w", err)
	}

	// Create CA issuer for second server
	if err := r.Create(ctx, resources.RecoveryServerCAIssuer); err != nil {
		return fmt.Errorf("failed to create recovery server CA issuer: %w", err)
	}

	// Create second server's certificate and recovery user certificate
	if err := r.Create(ctx, resources.RecoveryServerCertificate); err != nil {
		return fmt.Errorf("failed to create recovery server certificate: %w", err)
	}
	if err := r.Create(ctx, resources.RecoveryUserCertificate); err != nil {
		return fmt.Errorf("failed to create recovery user certificate: %w", err)
	}

	// Wait for server certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, resources.RecoveryServerCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server certificate not ready: %w", err)
	}

	// Wait for user certificate to be ready
	err = wait.For(
		conditions.CertificateIsReady(r, resources.RecoveryUserCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery user certificate not ready: %w", err)
	}

	// Create second Klio Server (shares tier2 storage with first server)
	if err := r.Create(ctx, resources.RecoveryServer); err != nil {
		return fmt.Errorf("failed to create recovery server: %w", err)
	}

	// Wait for second Server to be ready
	err = wait.For(
		conditions.KlioServerIsReady(r, resources.RecoveryServer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("recovery server not ready: %w", err)
	}

	// Create PluginConfiguration for recovery (points to second server)
	if err := r.Create(ctx, resources.PluginConfigurationRecovery); err != nil {
		return fmt.Errorf("failed to create recovery plugin configuration: %w", err)
	}

	return nil
}

// tier2ScenarioResources contains all resources created by buildTier2ScenarioResources.
type tier2ScenarioResources struct {
	// Common resources
	Namespace *corev1.Namespace
	Issuer    *certmanagerv1.Issuer

	// RustFS infrastructure
	RustfsSecret          *corev1.Secret
	RustfsConfigMap       *corev1.ConfigMap
	RustfsPVC             *corev1.PersistentVolumeClaim
	RustfsLogsPVC         *corev1.PersistentVolumeClaim
	RustfsCertificate     *certmanagerv1.Certificate
	RustfsService         *corev1.Service
	RustfsDeployment      *appsv1.Deployment
	RustfsCreateBucketJob *batchv1.Job

	// Klio Server with tier2
	ServerCertificate *certmanagerv1.Certificate
	CACertificate     *certmanagerv1.Certificate
	CAIssuer          *certmanagerv1.Issuer
	UserCertificate   *certmanagerv1.Certificate
	EncryptionSecret  *corev1.Secret
	IdentitySecret    *corev1.Secret
	KlioServer        *kliov1alpha1.Server

	// Source cluster
	CNPGCluster                   *cnpgv1.Cluster
	KlioPluginConfigurationSource *kliov1alpha1.PluginConfiguration

	// Recovery resources (created after tier2 replication)
	RecoveryServerCertificate       *certmanagerv1.Certificate
	RecoveryServerCACertificate     *certmanagerv1.Certificate
	RecoveryServerCAIssuer          *certmanagerv1.Issuer
	RecoveryUserCertificate         *certmanagerv1.Certificate
	RecoveryServer                  *kliov1alpha1.Server
	KlioPluginConfigurationRecovery *kliov1alpha1.PluginConfiguration

	// Test resources
	Backup          *cnpgv1.Backup
	RecoveryCluster *cnpgv1.Cluster
}

// buildTier2ScenarioResources creates all resources needed for tier2 recovery testing.
// This function is shared between tier2 recovery and tier2 PITR tests.
//
//nolint:funlen
func buildTier2ScenarioResources(namespace string, instances int) *tier2ScenarioResources {
	// Namespace
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	// Issuer for all certificates
	issuer := certificates.GetSelfSignedIssuerObject(tier2SelfSignedIssuerName, namespace)

	// RustFS infrastructure
	rustfsSecretObj := rustfs.GetRustFSSecret(tier2RustfsSecretName, namespace)
	rustfsConfigMapObj := rustfs.GetRustFSConfigMap(tier2RustfsConfigMapName, namespace)
	rustfsPVC := rustfs.GetRustFSPVC(tier2RustfsDataPVCName, namespace)
	rustfsLogsPVCObj := rustfs.GetRustFSLogsPVC(tier2RustfsLogsPVCName, namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(tier2RustfsResourceName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(tier2RustfsResourceName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(tier2RustfsResourceName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		tier2RustfsCreateBucketJobName, namespace, rustfs.RustFSBucketName)

	// Klio Server certificates and secrets
	caCertificate := certificates.GetCACertificateObject(tier2CACertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(tier2CAIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(tier2ServerCertificateName, namespace,
		[]string{tier2KlioServerName}, issuer)
	userCertificate := certificates.GetUserCertificateObject(
		tier2SourceClientCertName, namespace, tier2SourceClientCertName+"@"+tier2SourceClusterName, caIssuer)

	// Encryption secret (MUST be same for tier1 and tier2)
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(tier2EncryptionSecretName, namespace, tier2EncryptionPassword)

	// Klio Server with tier2
	encOpts := klio.EncryptionOptions{
		EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
		EncryptionKeyFileName:   "encryption-key.age",
		IdentitySecretName:      ageSecrets.IdentitySecret.Name,
		IdentityFileName:        "identity.txt",
	}

	klioServer := klio.GetServerWithTier2Object(
		tier2KlioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				TLSSecretName:      serverCertificate.Spec.SecretName,
				ClientCASecretName: caCertificate.Spec.SecretName,
				Encryption:         encOpts,
			},
			Tier2Encryption: encOpts, // Same encryption key for tier1 and tier2
			S3: klio.Tier2S3Options{
				S3BucketName:          rustfs.RustFSBucketName,
				S3Prefix:              tier2S3Prefix,
				S3Endpoint:            rustfs.GetRustFSEndpoint(tier2RustfsResourceName, namespace),
				S3Region:              rustfs.RustFSRegion,
				S3AccessKeySecretName: rustfsSecretObj.Name,
				S3SecretKeySecretName: rustfsSecretObj.Name,
				S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
			},
		},
	)

	// Source CNPG cluster
	cnpgCluster := cnpg.GetCnpgClusterObject(
		tier2SourceClusterName, namespace, instances, tier2SourcePluginConfigName)

	// Plugin configuration for source cluster (with tier2 backup enabled)
	klioPluginConfigurationSource := klio.GetPluginConfigurationObject(
		tier2SourcePluginConfigName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			ClusterName:         tier2SourceClusterName,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			Mode:                kliov1alpha1.ModeStandard,
		},
	)

	// Backup
	backup := cnpg.GetCnpgBackupObject(tier2BackupName, namespace, cnpgv1.DefaultBackupTarget, cnpgCluster)

	// Second Klio Server (to be created after tier2 replication)
	recoveryServerCACertificate := certificates.GetCACertificateObject(tier2RecoveryServerCACertName, namespace, issuer)
	recoveryServerCAIssuer := certificates.GetCAIssuerObject(tier2RecoveryServerCAIssuerName, namespace,
		recoveryServerCACertificate.Spec.SecretName)
	recoveryServerCertificate := certificates.GetCertificateObject(
		tier2RecoveryServerCertName, namespace, []string{tier2KlioReadOnlyServerName}, recoveryServerCAIssuer)
	recoveryUserCertificate := certificates.GetUserCertificateObject(
		tier2RestoredClientCertName, namespace, tier2SourceClientCertName+"@"+tier2SourceClusterName,
		recoveryServerCAIssuer)

	recoveryServer := klio.GetReadOnlyTier2ServerObject(
		tier2KlioReadOnlyServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				TLSSecretName:      recoveryServerCertificate.Spec.SecretName,
				ClientCASecretName: recoveryServerCACertificate.Spec.SecretName,
				Encryption:         encOpts, // SAME as first server
			},
			Tier2Encryption: encOpts, // SAME as first server
			S3: klio.Tier2S3Options{
				S3BucketName:          rustfs.RustFSBucketName,
				S3Prefix:              tier2S3Prefix, // SAME as first server
				S3Endpoint:            rustfs.GetRustFSEndpoint(tier2RustfsResourceName, namespace),
				S3Region:              rustfs.RustFSRegion,
				S3AccessKeySecretName: rustfsSecretObj.Name,
				S3SecretKeySecretName: rustfsSecretObj.Name,
				S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
			},
		},
	)

	// Plugin configuration for recovery cluster (with tier2 recovery enabled)
	klioPluginConfigurationRecovery := klio.GetPluginConfigurationObject(
		tier2RecoveryPluginConfigName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   recoveryServerCertificate,
			ClientCertificate:   recoveryUserCertificate,
			ClusterName:         tier2SourceClusterName,
			EnableTier2Backup:   false,
			EnableTier2Recovery: true,
			Mode:                kliov1alpha1.ModeReadOnly,
		},
	)

	// Recovery cluster configuration
	recoveryCluster := cnpgCluster.DeepCopy()
	recoveryCluster.Name = tier2RestoredClusterName
	recoveryCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: tier2SourceClusterName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: new(true),
			Parameters: map[string]string{
				klioconfig.PluginConfigurationRefParam: klioPluginConfigurationRecovery.Name,
			},
		},
	}}
	recoveryCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: tier2SourceClusterName,
		},
	}
	recoveryCluster.Spec.Plugins = []cnpgv1.PluginConfiguration{}

	return &tier2ScenarioResources{
		Namespace:                       namespaceObj,
		Issuer:                          issuer,
		RustfsSecret:                    rustfsSecretObj,
		RustfsConfigMap:                 rustfsConfigMapObj,
		RustfsPVC:                       rustfsPVC,
		RustfsLogsPVC:                   rustfsLogsPVCObj,
		RustfsCertificate:               rustfsCertificate,
		RustfsService:                   rustfsService,
		RustfsDeployment:                rustfsDeployment,
		RustfsCreateBucketJob:           rustfsCreateBucketJob,
		ServerCertificate:               serverCertificate,
		CACertificate:                   caCertificate,
		CAIssuer:                        caIssuer,
		UserCertificate:                 userCertificate,
		EncryptionSecret:                ageSecrets.EncryptionKeySecret,
		IdentitySecret:                  ageSecrets.IdentitySecret,
		KlioServer:                      klioServer,
		CNPGCluster:                     cnpgCluster,
		KlioPluginConfigurationSource:   klioPluginConfigurationSource,
		RecoveryServerCertificate:       recoveryServerCertificate,
		RecoveryServerCACertificate:     recoveryServerCACertificate,
		RecoveryServerCAIssuer:          recoveryServerCAIssuer,
		RecoveryUserCertificate:         recoveryUserCertificate,
		RecoveryServer:                  recoveryServer,
		KlioPluginConfigurationRecovery: klioPluginConfigurationRecovery,
		Backup:                          backup,
		RecoveryCluster:                 recoveryCluster,
	}
}

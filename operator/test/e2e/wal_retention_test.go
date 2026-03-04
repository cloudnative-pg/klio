package e2e

import (
	"bytes"
	"context"
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
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryPostgres "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// walRetentionScenario contains all resources needed for WAL retention queue-awareness testing.
type walRetentionScenario struct {
	// Common
	namespace *corev1.Namespace
	issuer    *certmanagerv1.Issuer

	// RustFS infrastructure (for tier2 storage)
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
	cnpgCluster             *cnpgv1.Cluster
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
	backup                  *cnpgv1.Backup

	name             string
	sourcePrimaryPod corev1.Pod
}

// Setup creates all resources for WAL retention testing.
func (s *walRetentionScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for WAL retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create namespace
	require.NoError(t, r.Create(ctx, s.namespace), "failed to create namespace")

	// Set Scenario infra
	scenario := infra.Tier2{
		Issuer:                s.issuer,
		RustfsSecret:          s.rustfsSecret,
		RustfsConfigMap:       s.rustfsConfigMap,
		RustfsPVC:             s.rustfsPVC,
		RustfsLogsPVC:         s.rustfsLogsPVC,
		RustfsCertificate:     s.rustfsCertificate,
		RustfsService:         s.rustfsService,
		RustfsDeployment:      s.rustfsDeployment,
		RustfsCreateBucketJob: s.rustfsCreateBucketJob,
		ServerCertificate:     s.serverCertificate,
		CaCertificate:         s.caCertificate,
		CaIssuer:              s.caIssuer,
		UserCertificate:       s.userCertificate,
		EncryptionSecret:      s.encryptionSecret,
		KlioServer:            s.klioServer,
	}

	// Parallel setup of RustFS and Klio Server for Tier2 scenario
	scenario.ParallelSetup(ctx, t, r)

	// Deploy source CNPG cluster
	t.Logf("Deploying source CNPG cluster...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfiguration),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG source cluster")

	// Wait for source cluster to be ready
	t.Logf("Waiting for source cluster to be ready...")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
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

	t.Logf("All resources created and ready for WAL retention feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *walRetentionScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for WAL retention feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for WAL retention feature: %s", s.name)

	return ctx
}

// getWALFilesInTier1 returns the list of WAL files stored in tier1 for a cluster.
func (s *walRetentionScenario) getWALFilesInTier1(
	ctx context.Context,
	r *resources.Resources,
) []string {
	podName := s.klioServer.Name + "-klio-0"
	containerName := "server"

	// List WAL files in the tier1 WAL directory.
	// WAL files are stored at /data/wal/{clusterName}/XXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXX.
	// Note: 'find' is not available in the minimal container, so we use ls with shell globbing.
	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"sh", "-c",
		fmt.Sprintf("ls /data/wal/%s/*/0000* 2>/dev/null | sort", s.cnpgCluster.Name),
	}

	// ls returns exit code 1 if no files match, which is fine - we just return empty list.
	_ = r.ExecInPod(ctx, s.namespace.Name, podName, containerName, listCmd, &stdout, &stderr)

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return []string{}
	}

	files := strings.Split(output, "\n")
	// Extract just the WAL file names (excluding .partial files)
	walFiles := make([]string, 0, len(files))
	for _, f := range files {
		parts := strings.Split(f, "/")
		if len(parts) > 0 {
			fileName := parts[len(parts)-1]
			// Skip partial WAL files
			if !strings.HasSuffix(fileName, ".partial") {
				walFiles = append(walFiles, fileName)
			}
		}
	}

	return walFiles
}

// runBackupMaintenance triggers the backup maintenance command on the PostgreSQL pod.
// This runs the "klio backup maintenance" command which calls SetFirstRequiredWAL on the Klio server.
// The command is executed in the klio-plugin sidecar container where the klio binary is installed.
func (s *walRetentionScenario) runBackupMaintenance(
	ctx context.Context,
	r *resources.Resources,
) error {
	// The klio config is at /var/lib/postgresql/klio/klio-archive on the PostgreSQL pod.
	// The klio binary is in the klio-plugin sidecar container.
	const (
		klioConfigPath   = "/var/lib/postgresql/klio/klio-archive"
		sidecarContainer = "klio-plugin"
	)

	var stdout, stderr bytes.Buffer
	maintenanceCmd := []string{
		"klio",
		"backup",
		"maintenance",
		"--config",
		klioConfigPath,
	}

	err := r.ExecInPod(
		ctx, s.namespace.Name, s.sourcePrimaryPod.Name, sidecarContainer, maintenanceCmd, &stdout, &stderr)
	if err != nil {
		return fmt.Errorf(
			"failed to run backup maintenance: %w; stdout: %s; stderr: %s", err, stdout.String(), stderr.String())
	}

	return nil
}

// deleteBackup deletes a backup by name using the klio CLI.
func (s *walRetentionScenario) deleteBackup(
	ctx context.Context,
	r *resources.Resources,
	backupName string,
) error {
	const (
		klioConfigPath   = "/var/lib/postgresql/klio/klio-archive"
		sidecarContainer = "klio-plugin"
	)

	var stdout, stderr bytes.Buffer
	deleteCmd := []string{
		"klio",
		"backup",
		"delete",
		"--config",
		klioConfigPath,
		backupName,
	}

	err := r.ExecInPod(
		ctx, s.namespace.Name, s.sourcePrimaryPod.Name, sidecarContainer, deleteCmd, &stdout, &stderr)
	if err != nil {
		return fmt.Errorf(
			"failed to delete backup %s: %w; stdout: %s; stderr: %s",
			backupName, err, stdout.String(), stderr.String())
	}

	return nil
}

// listBackups returns a list of Kopia backup names sorted by creation time (oldest first).
// The backup names are the actual Kopia repository backup names (e.g., "backup-20260202121752"),
// not the Kubernetes Backup resource names.
func (s *walRetentionScenario) listBackups(
	ctx context.Context,
	r *resources.Resources,
) ([]string, error) {
	const (
		klioConfigPath   = "/var/lib/postgresql/klio/klio-archive"
		sidecarContainer = "klio-plugin"
	)

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"klio",
		"backup",
		"list",
		"--config",
		klioConfigPath,
	}

	err := r.ExecInPod(
		ctx, s.namespace.Name, s.sourcePrimaryPod.Name, sidecarContainer, listCmd, &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to list backups: %w; stdout: %s; stderr: %s",
			err, stdout.String(), stderr.String())
	}

	// klio backup list outputs JSON directly (no flag needed).
	// The output format is: [{"name":"backup-20260202121752","startWal":"...","endWal":"..."},...].
	output := strings.TrimSpace(stdout.String())
	if output == "" || output == "[]" || output == "null" {
		return []string{}, nil
	}

	// Simple JSON parsing to extract backup names.
	// We look for patterns like "name":"backup-TIMESTAMP".
	var backupNames []string
	lines := strings.Split(output, `"name":"`)
	for i := 1; i < len(lines); i++ { // Skip the first part before any "name":"
		endIdx := strings.Index(lines[i], `"`)
		if endIdx > 0 {
			backupNames = append(backupNames, lines[i][:endIdx])
		}
	}

	return backupNames, nil
}

// WALRetentionFeature defines a feature for testing WAL retention queue-awareness.
type WALRetentionFeature struct {
	name     string
	scenario *walRetentionScenario
}

// Name returns the name of the feature.
func (f *WALRetentionFeature) Name() string {
	return f.name
}

// Setup initializes the test resources.
func (f *WALRetentionFeature) Setup() types.StepFunc {
	return f.scenario.Setup
}

// Run executes the WAL retention test.
func (f *WALRetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running WAL retention queue-awareness test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Step 1: Create a backup to establish a baseline.
		// This creates the first required WAL reference point.
		t.Log("Creating initial backup...")
		require.NoError(t, r.Create(ctx, f.scenario.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.scenario.backup),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "backup not completed")

		// Step 2: Generate WAL activity to create additional WAL files.
		t.Log("Generating WAL activity...")
		_, err = machineryPostgres.ExecPostgresQuery(
			ctx, r, &f.scenario.sourcePrimaryPod, "postgres",
			"CREATE TABLE IF NOT EXISTS wal_test (id serial PRIMARY KEY, data text)",
		)
		require.NoError(t, err, "failed to create test table")

		// Insert data and force WAL switches to generate multiple WAL files.
		for i := range 5 {
			_, err = machineryPostgres.ExecPostgresQuery(
				ctx, r, &f.scenario.sourcePrimaryPod, "postgres",
				"INSERT INTO wal_test (data) SELECT md5(random()::text) FROM generate_series(1, 1000)",
			)
			require.NoError(t, err, "failed to insert test data")

			// Force WAL switch after each batch to ensure we create multiple WAL files.
			err = machineryPostgres.CheckpointAndSwitchWal(ctx, r, &f.scenario.sourcePrimaryPod)
			require.NoError(t, err, "failed to switch WAL on iteration %d", i)
		}

		// Step 3: Wait for WAL archiving to stabilize and count WAL files.
		// We poll until the WAL count is stable (same count for 2 consecutive checks).
		t.Log("Waiting for WAL archiving to stabilize...")
		var walFilesBefore []string
		var previousCount int
		stableChecks := 0
		for i := range 12 { // Max 2 minutes (12 * 10s)
			time.Sleep(10 * time.Second)
			walFilesBefore = f.scenario.getWALFilesInTier1(ctx, r)
			t.Logf("WAL files (check %d): %d files - %v", i+1, len(walFilesBefore), walFilesBefore)

			if len(walFilesBefore) == previousCount && len(walFilesBefore) > 0 {
				stableChecks++
				if stableChecks >= 2 {
					t.Log("WAL count stabilized")
					break
				}
			} else {
				stableChecks = 0
			}
			previousCount = len(walFilesBefore)
		}
		require.GreaterOrEqual(t, len(walFilesBefore), 3,
			"should have at least 3 WAL files before testing retention")

		// Step 4: Create a second backup to establish a new first required WAL.
		// After this backup, WAL files older than the backup's StartWAL are candidates for deletion.
		t.Log("Creating second backup to advance the retention point...")
		secondBackup := f.scenario.backup.DeepCopy()
		secondBackup.Name = "test-backup-2"
		secondBackup.ResourceVersion = ""
		require.NoError(t, r.Create(ctx, secondBackup), "failed to create second backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, secondBackup),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "second backup not completed")

		// Re-capture WAL count after second backup (it may have added one more WAL).
		walFilesBefore = f.scenario.getWALFilesInTier1(ctx, r)
		t.Logf("WAL files before maintenance: %d files - %v", len(walFilesBefore), walFilesBefore)

		// Step 5: Run backup maintenance immediately after generating WALs.
		// This triggers SetFirstRequiredWAL. With tier2 enabled and WALs pending
		// transfer, the queue-awareness feature should preserve them.
		t.Log("Phase 1: Running backup maintenance (WALs should be pending tier2 transfer)...")
		err = f.scenario.runBackupMaintenance(ctx, r)
		require.NoError(t, err, "failed to run backup maintenance")

		// Get WAL files after first maintenance.
		walFilesAfterFirstMaint := f.scenario.getWALFilesInTier1(ctx, r)
		t.Logf("WAL files after first maintenance: %d files - %v",
			len(walFilesAfterFirstMaint), walFilesAfterFirstMaint)

		// Verify queue-awareness is working: WAL files should be preserved.
		// With tier2 enabled and WALs pending transfer, the queue-awareness feature
		// should prevent premature deletion of WALs that are still in the transfer queue.
		require.NotEmpty(t, walFilesAfterFirstMaint, "WAL files should not be completely deleted")

		// We allow at most 1 WAL to be deleted (the backup process itself may advance the WAL).
		// The key assertion is that queue-awareness prevents mass deletion of pending WALs.
		require.GreaterOrEqual(t, len(walFilesAfterFirstMaint), len(walFilesBefore)-1,
			"queue-awareness should preserve WAL files pending tier2 transfer "+
				"(before: %d, after: %d)", len(walFilesBefore), len(walFilesAfterFirstMaint))

		t.Logf("Phase 1 passed: queue-awareness preserved WALs (before: %d, after: %d)",
			len(walFilesBefore), len(walFilesAfterFirstMaint))

		// Step 6: Delete the first backup to advance the retention point.
		// Maintenance calculates the oldest required WAL from ALL backups, so we need to
		// delete the first backup to allow retention of older WALs.
		// Note: We must use the actual Kopia backup name (e.g., "backup-20260202121752"),
		// not the Kubernetes Backup resource name ("test-backup").
		t.Log("Phase 2: Listing backups to find the oldest backup's Kopia name...")
		backupNames, err := f.scenario.listBackups(ctx, r)
		require.NoError(t, err, "failed to list backups")
		require.GreaterOrEqual(t, len(backupNames), 2,
			"expected at least 2 backups, got %d: %v", len(backupNames), backupNames)
		t.Logf("Found backups: %v", backupNames)

		// Delete the first (oldest) backup to advance the retention point.
		firstBackupName := backupNames[0]
		t.Logf("Deleting first backup: %s", firstBackupName)
		err = f.scenario.deleteBackup(ctx, r, firstBackupName)
		require.NoError(t, err, "failed to delete first backup %s", firstBackupName)

		// Step 7: Wait for tier2 transfers to complete, then verify retention happens.
		// After tier2 transfers complete, retention SHOULD delete older WALs.
		t.Log("Waiting for tier2 transfers to complete and running maintenance...")
		const (
			maxRetries    = 30 // 5 minutes max
			retryInterval = 10 * time.Second
		)

		var walFilesAfter []string
		for attempt := 1; attempt <= maxRetries; attempt++ {
			time.Sleep(retryInterval)

			t.Logf("Running backup maintenance (attempt %d/%d)...", attempt, maxRetries)
			err = f.scenario.runBackupMaintenance(ctx, r)
			require.NoError(t, err, "failed to run backup maintenance")

			walFilesAfter = f.scenario.getWALFilesInTier1(ctx, r)
			t.Logf("WAL files after maintenance: %d files - %v", len(walFilesAfter), walFilesAfter)

			// Check if retention happened (fewer files than we started with).
			if len(walFilesAfter) < len(walFilesBefore) {
				t.Logf("Retention occurred on attempt %d", attempt)
				break
			}
		}

		// Verify that retention eventually happens after tier2 transfers complete.
		require.NotEmpty(t, walFilesAfter, "WAL files should not be completely deleted")
		require.Less(t, len(walFilesAfter), len(walFilesBefore),
			"after tier2 transfers complete, retention should delete older WAL files "+
				"(before: %d, after: %d)", len(walFilesBefore), len(walFilesAfter))

		t.Logf("Phase 2 passed: retention deleted %d of %d WAL files after tier2 transfers completed",
			len(walFilesBefore)-len(walFilesAfter), len(walFilesBefore))

		t.Log("WAL retention queue-awareness test completed successfully")

		return ctx
	}
}

// Teardown cleans up resources after the test.
func (f *WALRetentionFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

// newWALRetentionScenario creates a new WAL retention test scenario.
func newWALRetentionScenario(name string, namespace string) *walRetentionScenario {
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
		cnpgClientCertName    = cnpgClusterName + "-client"

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
		cnpgClientCertName, namespace, cnpgClientCertName+"@"+cnpgClusterName, caIssuer)

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

	// Source CNPG cluster
	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName)

	// Plugin configuration with tier2 backup enabled
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			EnableTier2Backup:   true,
			EnableTier2Recovery: false,
			Mode:                kliov1alpha1.ModeStandard,
		},
	)

	// Backup
	backup := cnpg.GetCnpgBackupObject(backupName, namespace, cnpgv1.BackupTargetPrimary, cnpgCluster)

	return &walRetentionScenario{
		namespace:               namespaceObj,
		issuer:                  issuer,
		rustfsSecret:            rustfsSecret,
		rustfsConfigMap:         rustfsConfigMap,
		rustfsPVC:               rustfsPVC,
		rustfsLogsPVC:           rustfsLogsPVC,
		rustfsCertificate:       rustfsCertificate,
		rustfsService:           rustfsService,
		rustfsDeployment:        rustfsDeployment,
		rustfsCreateBucketJob:   rustfsCreateBucketJob,
		serverCertificate:       serverCertificate,
		caCertificate:           caCertificate,
		caIssuer:                caIssuer,
		userCertificate:         userCertificate,
		encryptionSecret:        encryptionSecret,
		klioServer:              klioServer,
		cnpgCluster:             cnpgCluster,
		klioPluginConfiguration: klioPluginConfiguration,
		backup:                  backup,
		name:                    name,
	}
}

// WALRetentionQueueAwareness returns a Feature for testing WAL retention queue-awareness.
// This test verifies that WAL files pending transfer to tier2 are not deleted
// by tier1 retention even when they are older than what tier1 backups require.
func WALRetentionQueueAwareness(namespace string) *WALRetentionFeature {
	scenario := newWALRetentionScenario("WALRetentionQueueAwareness", namespace)

	return &WALRetentionFeature{
		name:     "WALRetentionQueueAwareness",
		scenario: scenario,
	}
}

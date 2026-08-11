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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
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
	createNamespace(ctx, t, r, s.namespace)

	// Set Scenario infra
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
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for WAL retention feature: %s", s.name)

	return ctx
}

// getWALFilesInTier1 returns the list of WAL files stored in tier1 for a cluster.
func (s *walRetentionScenario) getWALFilesInTier1(
	ctx context.Context,
	r *resources.Resources,
) ([]string, error) {
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
	err := r.ExecInPod(ctx, s.namespace.Name, podName, containerName, listCmd, &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in pod %s, container %s: %w", podName, containerName, err)
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return []string{}, fmt.Errorf(
			"no WAL files found in tier1 for cluster %s; stdout: %s; stderr: %s",
			s.cnpgCluster.Name,
			stdout.String(),
			stderr.String(),
		)
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

	return walFiles, nil
}

// deleteBackup deletes a backup by name using the klio CLI.
func (s *walRetentionScenario) deleteBackup(
	ctx context.Context,
	r *resources.Resources,
	backupName string,
) error {
	const klioConfigPath = "/var/lib/postgresql/klio/klio-archive"

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
		ctx, s.namespace.Name, s.sourcePrimaryPod.Name, cnpgi.KlioPluginContainerName, deleteCmd, &stdout, &stderr)
	if err != nil {
		return fmt.Errorf(
			"failed to delete backup %s: %w; stdout: %s; stderr: %s",
			backupName, err, stdout.String(), stderr.String())
	}

	return nil
}

// kopiaBackupInfo mirrors the subset of klioclient.BackupMetadata fields
// needed to identify and order the backups printed by "klio backup list".
type kopiaBackupInfo struct {
	Name      string `json:"name"`
	StartedAt int64  `json:"startedAt"`
}

// listBackups returns a list of Kopia backup names sorted by creation time (oldest first).
// The backup names are the actual Kopia repository backup names (e.g., "backup-20260202121752"),
// not the Kubernetes Backup resource names.
func (s *walRetentionScenario) listBackups(
	ctx context.Context,
	r *resources.Resources,
) ([]string, error) {
	const klioConfigPath = "/var/lib/postgresql/klio/klio-archive"

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"klio",
		"backup",
		"list",
		"--config",
		klioConfigPath,
	}

	err := r.ExecInPod(
		ctx, s.namespace.Name, s.sourcePrimaryPod.Name, cnpgi.KlioPluginContainerName, listCmd, &stdout, &stderr)
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

	var backups []kopiaBackupInfo
	if err := json.Unmarshal([]byte(output), &backups); err != nil {
		return nil, fmt.Errorf("failed to parse backup list %q: %w", output, err)
	}

	// The ordering "klio backup list" returns isn't guaranteed to be
	// chronological (it reflects however the underlying Kopia repository
	// enumerates snapshots), and callers rely on element 0 being the oldest
	// backup, so sort explicitly rather than trusting that order.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].StartedAt < backups[j].StartedAt
	})

	backupNames := make([]string, 0, len(backups))
	for _, b := range backups {
		backupNames = append(backupNames, b.Name)
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
//
// Post-backup maintenance now runs server-side: each backup completion makes
// the backup queue consumer apply tier1 retention, clamped to the tier2
// transfer frontier so WALs still pending upload are never deleted. There is
// no client command to trigger maintenance any more, so the test drives it by
// completing backups and observes the on-disk effect.
func (f *WALRetentionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running server-side WAL retention queue-awareness test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Step 1: first backup establishes a retention baseline.
		t.Log("Creating initial backup...")
		require.NoError(t, r.Create(ctx, f.scenario.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.scenario.backup),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "backup not completed")

		// Step 2: generate WAL activity so there are older WAL segments that a
		// later retention pass could prune.
		t.Log("Generating WAL activity...")
		_, err = machineryPostgres.ExecPostgresQuery(
			ctx, r, &f.scenario.sourcePrimaryPod, "postgres",
			"CREATE TABLE IF NOT EXISTS wal_test (id serial PRIMARY KEY, data text)",
		)
		require.NoError(t, err, "failed to create test table")

		for i := range 5 {
			_, err = machineryPostgres.ExecPostgresQuery(
				ctx, r, &f.scenario.sourcePrimaryPod, "postgres",
				"INSERT INTO wal_test (data) SELECT md5(random()::text) FROM generate_series(1, 1000)",
			)
			require.NoError(t, err, "failed to insert test data")

			err = machineryPostgres.CheckpointAndSwitchWal(ctx, r, &f.scenario.sourcePrimaryPod)
			require.NoError(t, err, "failed to switch WAL on iteration %d", i)
		}

		// Step 3: second backup. Its completion triggers a server-side
		// maintenance pass. Once the older backup is deleted, this backup's
		// begin WAL becomes the retention frontier: retention may prune
		// everything older, but only after those WALs reach tier2.
		t.Log("Creating second backup...")
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

		var secondCompleted cnpgv1.Backup
		require.NoError(t, r.Get(ctx, secondBackup.Name, secondBackup.Namespace, &secondCompleted),
			"failed to get completed second backup")
		boundary := secondCompleted.Status.BeginWal
		require.NotEmpty(t, boundary, "second backup has no begin WAL in its status")

		// There must be WAL segments older than the second backup's begin WAL,
		// otherwise the retention assertion below would pass vacuously.
		walFiles, err := f.scenario.getWALFilesInTier1(ctx, r)
		require.NoError(t, err, "failed to get WAL files in tier1")
		t.Logf("Tier1 WAL files before retention advances: %d (%v), boundary %q", len(walFiles), walFiles, boundary)
		require.NotEmpty(t, walsOlderThan(walFiles, boundary),
			"expected WAL segments older than the second backup begin WAL before retention advances")

		// Step 4: delete the oldest backup so the retention point can advance to
		// the second backup's begin WAL.
		t.Log("Deleting the oldest backup to advance the retention point...")
		backupNames, err := f.scenario.listBackups(ctx, r)
		require.NoError(t, err, "failed to list backups")
		require.GreaterOrEqual(t, len(backupNames), 2,
			"expected at least 2 backups, got %d: %v", len(backupNames), backupNames)
		require.NoError(t, f.scenario.deleteBackup(ctx, r, backupNames[0]),
			"failed to delete oldest backup %s", backupNames[0])

		// Step 5: a third backup triggers a fresh maintenance pass now that the
		// oldest backup is gone and tier2 has had time to catch up.
		t.Log("Creating a third backup to trigger another maintenance pass...")
		thirdBackup := f.scenario.backup.DeepCopy()
		thirdBackup.Name = "test-backup-3"
		thirdBackup.ResourceVersion = ""
		require.NoError(t, r.Create(ctx, thirdBackup), "failed to create third backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, thirdBackup),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "third backup not completed")

		// Step 6: poll until server-side retention has pruned every WAL older
		// than the retention frontier. This only happens once those WALs have
		// been transferred to tier2, so it verifies both that retention runs
		// automatically and that the queue-awareness clamp releases after the
		// tier2 frontier advances.
		t.Log("Waiting for server-side tier1 WAL retention to prune WALs older than the retention frontier...")
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				var err error
				walFiles, err = f.scenario.getWALFilesInTier1(ctx, r)
				return len(walsOlderThan(walFiles, boundary)) == 0, err
			},
			wait.WithTimeout(5*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err,
			"server-side retention did not prune WALs older than boundary %q; remaining older WALs: %v",
			boundary, walsOlderThan(walFiles, boundary))
		require.NotEmpty(t, walFiles, "tier1 WAL repository unexpectedly empty after retention")

		t.Logf("Server-side WAL retention verified: %d WAL files remain, all >= begin WAL %q",
			len(walFiles), boundary)

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
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(encryptionSecretName, namespace, encryptionPassword)

	// Klio Server with tier2
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

	// Source CNPG cluster
	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})

	// Plugin configuration with tier2 backup enabled
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		pluginConfigurationName,
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate:   serverCertificate,
			ClientCertificate:   userCertificate,
			ClusterName:         cnpgClusterName,
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
		rustfsCertificate:       rustfsCertificate,
		rustfsService:           rustfsService,
		rustfsDeployment:        rustfsDeployment,
		rustfsCreateBucketJob:   rustfsCreateBucketJob,
		serverCertificate:       serverCertificate,
		caCertificate:           caCertificate,
		caIssuer:                caIssuer,
		userCertificate:         userCertificate,
		encryptionSecret:        ageSecrets.EncryptionKeySecret,
		identitySecret:          ageSecrets.IdentitySecret,
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

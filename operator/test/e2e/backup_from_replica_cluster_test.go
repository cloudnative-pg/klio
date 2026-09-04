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
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/postgres"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// ReplicaClusterBackupFeature verifies that an immediate backup taken from a
// freshly-created replica cluster completes. On a replica cluster the WAL
// streamer of the designated primary starts archiving from the current flush
// position, while pg_backup_start on the underlying standby reports the older
// last-restartpoint LSN: the WAL segments in between must still end up in tier1
// or the backup waits for WAL files that never arrive.
type ReplicaClusterBackupFeature struct {
	scenario *commonBackupRestoreScenario

	// sourceBackup is the base backup of the source cluster the replica
	// bootstraps from.
	sourceBackup *cnpgv1.Backup
	// replicaCluster is the replica cluster archiving to its own tier1.
	replicaCluster *cnpgv1.Cluster
	// replicaUserCertificate authenticates the replica cluster against the
	// Klio server under its own cluster name.
	replicaUserCertificate *certmanagerv1.Certificate
	// replicaPluginConfiguration wires the replica cluster to its own tier1.
	replicaPluginConfiguration *kliov1alpha1.PluginConfiguration
	// replicaBackup is the immediate backup taken from the replica cluster.
	replicaBackup *cnpgv1.Backup

	sourceBackupTimeout time.Duration
	recoveryTimeout     time.Duration
	// replicaBackupTimeout bounds the wait for the replica backup so a
	// never-arriving WAL fails the test instead of hanging.
	replicaBackupTimeout time.Duration
	checkInterval        time.Duration
}

// BackupFromReplicaCluster builds the "immediate backup from a replica cluster"
// feature: it backs up a source cluster, bootstraps a replica cluster that
// streams from it and archives to its own tier1, then takes an immediate backup
// of the replica cluster and asserts it completes.
func BackupFromReplicaCluster(namespace string) *ReplicaClusterBackupFeature {
	const (
		sourceClusterName  = "test-cluster-source"
		replicaClusterName = "test-cluster-replica"
		sourceExternalName = "source-cluster"
	)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	sourceCluster := cnpg.GetCnpgClusterObject(sourceClusterName, namespace, 1,
		"klio-plugin-configuration",
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})
	// Switch WAL frequently so the segments required by a backup are archived
	// promptly, and keep enough WAL around for the replica streamer to resume
	// from an older position.
	sourceCluster.Spec.PostgresConfiguration.Parameters = map[string]string{
		"archive_timeout": "30s",
		"wal_keep_size":   "512MB",
	}

	sourceUserCertificate := certificates.GetUserCertificateObject(
		"klio-user", namespace, "klio-user@"+sourceClusterName, caIssuer)
	sourcePluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: sourceUserCertificate,
			ClusterName:       sourceClusterName,
		},
	)
	// The replica reads the source's tier1 (same server, same cluster name) to
	// bootstrap and to stream from it.
	sourceExternalPluginConfiguration := sourcePluginConfiguration.DeepCopy()
	sourceExternalPluginConfiguration.Name = "klio-plugin-configuration-source"

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

	sourceBackup := cnpg.GetCnpgBackupObject("test-backup-source", namespace,
		cnpgv1.BackupTargetPrimary, sourceCluster)

	// The replica cluster archives to its own tier1 (its own cluster name and
	// client certificate).
	replicaUserCertificate := certificates.GetUserCertificateObject(
		"klio-user-replica", namespace, "klio-user@"+replicaClusterName, caIssuer)
	replicaPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration-replica",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: replicaUserCertificate,
			ClusterName:       replicaClusterName,
		},
	)

	replicaCluster := sourceCluster.DeepCopy()
	replicaCluster.Name = replicaClusterName
	replicaCluster.Spec.Plugins[0].Parameters[klioconfig.PluginConfigurationRefParam] = replicaPluginConfiguration.Name
	replicaCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
		Recovery: &cnpgv1.BootstrapRecovery{
			Source: sourceExternalName,
		},
	}
	replicaCluster.Spec.ReplicaCluster = &cnpgv1.ReplicaClusterConfiguration{
		Source:  sourceExternalName,
		Enabled: new(true),
	}
	replicaCluster.Spec.ExternalClusters = []cnpgv1.ExternalCluster{{
		Name: sourceExternalName,
		PluginConfiguration: &cnpgv1.PluginConfiguration{
			Name:    "klio.cnpg.io",
			Enabled: new(true),
			Parameters: map[string]string{
				klioconfig.PluginConfigurationRefParam: sourceExternalPluginConfiguration.Name,
			},
		},
	}}

	replicaBackup := cnpg.GetCnpgBackupObject("test-backup-replica", namespace,
		cnpgv1.BackupTargetPrimary, replicaCluster)

	scenario := &commonBackupRestoreScenario{
		namespace:                       namespaceObj,
		cnpgCluster:                     sourceCluster,
		userCertificate:                 sourceUserCertificate,
		encryptionSecret:                ageSecrets.EncryptionKeySecret,
		identitySecret:                  ageSecrets.IdentitySecret,
		issuer:                          issuer,
		caIssuer:                        caIssuer,
		caCertificate:                   caCertificate,
		certificate:                     certificate,
		klioServer:                      klioServer,
		klioPluginConfigurationSource:   sourcePluginConfiguration,
		klioPluginConfigurationRecovery: sourceExternalPluginConfiguration,
		name:                            "BackupFromReplicaCluster",
	}

	return &ReplicaClusterBackupFeature{
		scenario:                   scenario,
		sourceBackup:               sourceBackup,
		replicaCluster:             replicaCluster,
		replicaUserCertificate:     replicaUserCertificate,
		replicaPluginConfiguration: replicaPluginConfiguration,
		replicaBackup:              replicaBackup,
		sourceBackupTimeout:        2 * time.Minute,
		recoveryTimeout:            5 * time.Minute,
		replicaBackupTimeout:       3 * time.Minute,
		checkInterval:              10 * time.Second,
	}
}

// Name returns the feature name.
func (f *ReplicaClusterBackupFeature) Name() string {
	return f.scenario.name
}

// Setup creates the source cluster, the Klio server and the source-side plugin
// configurations, and waits for them to be ready.
func (f *ReplicaClusterBackupFeature) Setup() types.StepFunc {
	return f.scenario.Setup
}

// Run backs up the source cluster, bootstraps the replica cluster, then takes an
// immediate backup of the replica cluster and asserts it completes.
func (f *ReplicaClusterBackupFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running backup-from-replica-cluster feature test")
		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// Take a base backup of the source cluster so the replica can bootstrap.
		require.NoError(t, r.Create(ctx, f.sourceBackup), "failed to create source backup")
		require.NoError(t, wait.For(
			machineryConditions.BackupIsCompleted(r, f.sourceBackup),
			wait.WithTimeout(f.sourceBackupTimeout),
			wait.WithInterval(f.checkInterval),
		), "source backup not completed")

		// Advance the source WAL (without a checkpoint) so the replica, once
		// bootstrapped, replays past its last restartpoint: this is the state in
		// which the streamer starts ahead of what pg_backup_start reports.
		_, err = postgres.ExecPostgresQuery(ctx, r, &f.scenario.sourcePrimaryPod, "postgres",
			"CREATE TABLE numbers AS SELECT generate_series(1, 1000) AS x; "+
				"SELECT pg_switch_wal(); SELECT pg_switch_wal();")
		require.NoError(t, err, "failed to advance source WAL")

		// Create the replica-side archiving resources and the replica cluster.
		require.NoError(t, r.Create(ctx, f.replicaUserCertificate),
			"failed to create replica user certificate")
		require.NoError(t, r.Create(ctx, f.replicaPluginConfiguration),
			"failed to create replica plugin configuration")
		require.NoError(t, r.Create(ctx, f.replicaCluster), "failed to create replica cluster")
		require.NoError(t, wait.For(
			machineryConditions.ClusterIsReady(r, f.replicaCluster),
			wait.WithTimeout(f.recoveryTimeout),
			wait.WithInterval(f.checkInterval),
		), "replica cluster not ready")

		// The immediate backup of the freshly-created replica cluster must
		// complete: before the fix it loops forever on missing WAL files.
		require.NoError(t, r.Create(ctx, f.replicaBackup), "failed to create replica backup")
		require.NoError(t, wait.For(
			machineryConditions.BackupIsCompleted(r, f.replicaBackup),
			wait.WithTimeout(f.replicaBackupTimeout),
			wait.WithInterval(f.checkInterval),
		), "replica cluster backup not completed")

		return ctx
	}
}

// Teardown removes the resources created for the feature.
func (f *ReplicaClusterBackupFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

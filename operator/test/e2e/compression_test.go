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
	"strings"
	"testing"
	"time"

	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
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

const (
	// globalCompressionAlgorithm is the repository-wide compression policy set
	// on the Server. Clusters without an override inherit it.
	globalCompressionAlgorithm = "s2-default"

	// clusterCompressionAlgorithm is the per-cluster compression policy set on
	// the PluginConfiguration. It must differ from globalCompressionAlgorithm
	// so we can prove the override takes precedence over the global policy.
	clusterCompressionAlgorithm = "zstd"

	// globalCompressionMinSize and clusterCompressionMinSize exercise the
	// optional minSize bound on the Server (global) and the PluginConfiguration
	// (per-cluster) respectively. They differ so the override is provable.
	globalCompressionMinSize  = 2048
	clusterCompressionMinSize = 4096

	// tier1KopiaConfigPattern and tier2RWKopiaConfigPattern glob the Kopia
	// config files the server creates at startup, via their companion password
	// files. tier1 is the local filesystem repository; tier2 is the read-write
	// S3 repository.
	tier1KopiaConfigPattern   = "/tmp/kopiaconfig_tier1_*.kopia-password"
	tier2RWKopiaConfigPattern = "/tmp/kopiaconfig_tier2_rw_*.kopia-password"

	compressionServerContainerName = "server"
	compressionKlioPodSuffix       = "-klio-0"
)

// compressionScenario contains all resources needed for compression testing.
// It reuses the tier2 infrastructure so that both the tier2 global policy
// (Server) and the tier2 per-cluster policy (PluginConfiguration) can be
// verified against the same Kopia repository.
type compressionScenario struct {
	namespace *corev1.Namespace

	// tier2Infra bundles the shared RustFS + Klio Server (tier2) bring-up.
	tier2Infra infra.Tier2

	// klioServer is a shortcut to tier2Infra.KlioServer used by the verifier.
	klioServer *kliov1alpha1.Server

	// Source cluster
	cnpgCluster             *cnpgv1.Cluster
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
	backup                  *cnpgv1.Backup

	name string
}

// Setup creates all resources for compression testing.
func (s *compressionScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for compression feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	createNamespace(ctx, t, r, s.namespace)

	// Bring up the shared RustFS + tier2 Klio Server infrastructure.
	s.tier2Infra.ParallelSetup(ctx, t, r)

	t.Logf("Deploying source CNPG cluster and plugin configuration...")
	require.NoError(t, r.Create(ctx, s.klioPluginConfiguration),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG source cluster")

	require.NoError(t, wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	), "source cluster not ready")

	t.Logf("All resources ready for compression feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *compressionScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for compression feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for compression feature: %s", s.name)

	return ctx
}

// kopiaConfigFile discovers an ephemeral Kopia config file the server created
// at startup, by globbing its companion password file with the passed pattern.
func (s *compressionScenario) kopiaConfigFile(
	ctx context.Context,
	r *resources.Resources,
	pattern string,
) (string, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	findCmd := []string{"sh", "-c", "ls " + pattern + " 2>/dev/null"}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, findCmd, &stdout, &stderr,
	); err != nil {
		return "", fmt.Errorf("failed to locate kopia config %q: %w; stderr: %s", pattern, err, stderr.String())
	}

	passwordFile := strings.TrimSpace(stdout.String())
	if passwordFile == "" {
		return "", fmt.Errorf("no kopia config file found matching %q", pattern)
	}

	return strings.TrimSuffix(passwordFile, ".kopia-password"), nil
}

// compressionOfPolicy runs `kopia policy show <target> --json` against the
// passed config and returns the effective compression settings.
func (s *compressionScenario) compressionOfPolicy(
	ctx context.Context,
	r *resources.Resources,
	configFile string,
	target string,
) (effectiveCompression, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	showCmd := []string{
		"kopia", "policy", "show", target,
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, showCmd, &stdout, &stderr,
	); err != nil {
		return effectiveCompression{},
			fmt.Errorf("failed to show kopia policy for %q: %w; stderr: %s", target, err, stderr.String())
	}

	var policy struct {
		Compression effectiveCompression `json:"compression"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policy); err != nil {
		return effectiveCompression{}, fmt.Errorf("failed to parse kopia policy %q: %w", stdout.String(), err)
	}

	return policy.Compression, nil
}

// effectiveCompression is the subset of a Kopia policy's compression settings
// that the test asserts on.
type effectiveCompression struct {
	CompressorName string `json:"compressorName"`
	MinSize        int64  `json:"minSize"`
}

// clusterPolicyTarget finds the "user@host" policy target for the cluster in
// the tier2 repository. The per-cluster policy only exists once the backup has
// been relayed to tier2, so this returns an empty string until then.
func (s *compressionScenario) clusterPolicyTarget(
	ctx context.Context,
	r *resources.Resources,
	configFile string,
) (string, error) {
	podName := s.klioServer.Name + compressionKlioPodSuffix

	var stdout, stderr bytes.Buffer
	listCmd := []string{
		"kopia", "policy", "list",
		"--disable-file-logging",
		"--config-file=" + configFile,
		"--json",
	}
	if err := r.ExecInPod(
		ctx, s.namespace.Name, podName, compressionServerContainerName, listCmd, &stdout, &stderr,
	); err != nil {
		return "", fmt.Errorf("failed to list kopia policies: %w; stderr: %s", err, stderr.String())
	}

	var policies []struct {
		Target struct {
			Host string `json:"host"`
			User string `json:"userName"`
		} `json:"target"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &policies); err != nil {
		return "", fmt.Errorf("failed to parse kopia policy list %q: %w", stdout.String(), err)
	}

	for _, p := range policies {
		if p.Target.Host == s.cnpgCluster.Name && p.Target.User != "" {
			return p.Target.User + "@" + p.Target.Host, nil
		}
	}

	return "", nil
}

// CompressionFeature verifies the global and per-cluster compression policies.
type CompressionFeature struct {
	name     string
	scenario *compressionScenario
}

// Name returns the name of the feature.
func (f *CompressionFeature) Name() string {
	return f.name
}

// Setup initializes the test resources.
func (f *CompressionFeature) Setup() types.StepFunc {
	return f.scenario.Setup
}

// Run verifies that the repository-wide compression policy configured on the
// Server is applied globally, and that the per-cluster policy configured on the
// PluginConfiguration overrides it for the cluster's own source. Both tier1 and
// tier2 repositories are checked.
func (f *CompressionFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running compression policy test")

		// The per-cluster policies must differ from the global one, otherwise
		// the override assertions below would pass vacuously.
		require.NotEqual(t, globalCompressionAlgorithm, clusterCompressionAlgorithm,
			"test misconfigured: global and per-cluster algorithms must differ")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		// A backup is required so the base data reaches both tiers, which is
		// when the per-cluster compression policies are applied (tier1 by
		// `klio backup run`, tier2 by the consumer during the relay).
		t.Log("Creating backup with tier2 enabled...")
		require.NoError(t, r.Create(ctx, f.scenario.backup), "failed to create backup")
		err = wait.For(
			machineryConditions.BackupIsCompleted(r, f.scenario.backup),
			wait.WithTimeout(3*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "backup not completed")

		f.scenario.verifyTierCompression(ctx, t, r, "tier1", tier1KopiaConfigPattern)
		f.scenario.verifyTierCompression(ctx, t, r, "tier2", tier2RWKopiaConfigPattern)

		return ctx
	}
}

// verifyTierCompression asserts that, for the repository selected by
// configPattern, the global compression policy matches the Server setting and
// the cluster's own source policy matches the per-cluster override.
func (s *compressionScenario) verifyTierCompression(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	tier string,
	configPattern string,
) {
	t.Helper()

	configFile, err := s.kopiaConfigFile(ctx, r, configPattern)
	require.NoError(t, err, "[%s] failed to discover kopia config file", tier)

	// The global policy is applied when the server starts, so it is already
	// present. Verify its algorithm and minSize match what the Server requested.
	t.Logf("[%s] verifying the global compression policy...", tier)
	globalCompression, err := s.compressionOfPolicy(ctx, r, configFile, "--global")
	require.NoError(t, err, "[%s] failed to read the global compression policy", tier)
	require.Equal(t, globalCompressionAlgorithm, globalCompression.CompressorName,
		"[%s] unexpected global compression algorithm", tier)
	require.Equal(t, int64(globalCompressionMinSize), globalCompression.MinSize,
		"[%s] unexpected global compression minSize", tier)

	// The per-cluster policy is applied during the backup, so poll until it
	// appears with the expected algorithm and minSize.
	t.Logf("[%s] waiting for the per-cluster compression policy...", tier)
	var clusterCompression effectiveCompression
	err = wait.For(
		func(ctx context.Context) (bool, error) {
			target, err := s.clusterPolicyTarget(ctx, r, configFile)
			if err != nil || target == "" {
				return false, err
			}
			clusterCompression, err = s.compressionOfPolicy(ctx, r, configFile, target)
			if err != nil {
				return false, err
			}

			return clusterCompression.CompressorName == clusterCompressionAlgorithm &&
				clusterCompression.MinSize == clusterCompressionMinSize, nil
		},
		wait.WithTimeout(3*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err,
		"[%s] per-cluster compression policy did not become %q/minSize=%d (last seen %+v)",
		tier, clusterCompressionAlgorithm, clusterCompressionMinSize, clusterCompression)

	t.Logf("[%s] compression policies verified: global=%+v, cluster=%+v",
		tier, globalCompression, clusterCompression)
}

// Teardown cleans up resources after the test.
func (f *CompressionFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

// newCompressionScenario creates a new compression test scenario.
func newCompressionScenario(name string, namespace string) *compressionScenario {
	const (
		cnpgClusterName = "pg-compression"

		klioServerName = "klio"

		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		cnpgClientCertName    = cnpgClusterName + "-client"

		rustfsName                = "rustfs"
		rustfsSecretName          = rustfsName + "-secret"
		rustfsConfigMapName       = rustfsName + "-config"
		rustfsCreateBucketJobName = rustfsName

		encryptionSecretName = "encryption"
		encryptionPassword   = "testencryptionpassword123"

		pluginConfigurationName = "klio-plugin-configuration"

		backupName = "test-backup"

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
		cnpgClientCertName, namespace, cnpgClientCertName+"@"+cnpgClusterName, caIssuer)

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
	// Repository-wide (global) compression policy on the Server: every cluster
	// that does not override it inherits this policy.
	klioServer.Spec.Tier1.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: globalCompressionAlgorithm,
		MinSize:   globalCompressionMinSize,
	}
	klioServer.Spec.Tier2.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: globalCompressionAlgorithm,
		MinSize:   globalCompressionMinSize,
	}

	cnpgCluster := cnpg.GetCnpgClusterObject(
		cnpgClusterName, namespace, 1, pluginConfigurationName,
		cnpg.ClusterTemplateOptions{StorageClass: testCfg.StorageClass})

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
	// Per-cluster compression policy overriding the Server global policy for
	// this cluster's own source, on both tiers.
	klioPluginConfiguration.Spec.Tier1 = &kliov1alpha1.Tier1PluginConfiguration{
		Compression: &kliov1alpha1.CompressionPolicy{
			Algorithm: clusterCompressionAlgorithm,
			MinSize:   clusterCompressionMinSize,
		},
	}
	klioPluginConfiguration.Spec.Tier2.Compression = &kliov1alpha1.CompressionPolicy{
		Algorithm: clusterCompressionAlgorithm,
		MinSize:   clusterCompressionMinSize,
	}

	backup := cnpg.GetCnpgBackupObject(backupName, namespace, cnpgv1.BackupTargetPrimary, cnpgCluster)

	return &compressionScenario{
		namespace: namespaceObj,
		tier2Infra: infra.Tier2{
			Issuer:                issuer,
			RustfsSecret:          rustfsSecret,
			RustfsConfigMap:       rustfsConfigMap,
			RustfsCertificate:     rustfsCertificate,
			RustfsService:         rustfsService,
			RustfsDeployment:      rustfsDeployment,
			RustfsCreateBucketJob: rustfsCreateBucketJob,
			ServerCertificate:     serverCertificate,
			CaCertificate:         caCertificate,
			CaIssuer:              caIssuer,
			UserCertificate:       userCertificate,
			EncryptionSecret:      ageSecrets.EncryptionKeySecret,
			IdentitySecret:        ageSecrets.IdentitySecret,
			KlioServer:            klioServer,
		},
		klioServer:              klioServer,
		cnpgCluster:             cnpgCluster,
		klioPluginConfiguration: klioPluginConfiguration,
		backup:                  backup,
		name:                    name,
	}
}

// Compression returns a Feature that verifies the global and per-cluster
// Kopia compression policies are applied and that the per-cluster policy
// overrides the global one.
func Compression(namespace string) *CompressionFeature {
	return &CompressionFeature{
		name:     "Compression",
		scenario: newCompressionScenario("Compression", namespace),
	}
}

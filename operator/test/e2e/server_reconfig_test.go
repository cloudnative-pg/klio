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
	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// serverReconfigScenario contains all resources needed for server tier reconfiguration testing.
type serverReconfigScenario struct {
	name      string
	namespace *corev1.Namespace

	// Issuer
	issuer *certmanagerv1.Issuer

	// RustFS infrastructure (needed for tier2 S3)
	rustfsSecret          *corev1.Secret
	rustfsConfigMap       *corev1.ConfigMap
	rustfsCertificate     *certmanagerv1.Certificate
	rustfsService         *corev1.Service
	rustfsDeployment      *appsv1.Deployment
	rustfsCreateBucketJob *batchv1.Job

	// Klio Server certificates and secrets
	serverCertificate *certmanagerv1.Certificate
	caCertificate     *certmanagerv1.Certificate
	caIssuer          *certmanagerv1.Issuer
	userCertificate   *certmanagerv1.Certificate
	encryptionSecret  *corev1.Secret
	identitySecret    *corev1.Secret

	// Klio Server (initially tier1 only, no tier2)
	klioServer *kliov1alpha1.Server

	// Tier2 configuration to add during Run
	s3Opts          klio.Tier2S3Options
	tier2Encryption klio.EncryptionOptions
	storageClass    string
}

// Setup creates all resources for the server reconfiguration test.
func (s *serverReconfigScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for server reconfiguration feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create namespace
	createNamespace(ctx, t, r, s.namespace)

	// Parallel setup of RustFS and Klio Server
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

	t.Logf("All resources created and ready for server reconfiguration feature: %s", s.name)

	return ctx
}

// Teardown deletes all resources.
func (s *serverReconfigScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for server reconfiguration feature: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	namespaces.DumpNamespaceOnFailure(ctx, t, r, testCfg.LogDir, s.namespace.Name, testconfig.DumpedKinds())
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for server reconfiguration feature: %s", s.name)

	return ctx
}

// serverReconfigFeature implements the Feature interface for server tier reconfiguration testing.
type serverReconfigFeature struct {
	name     string
	scenario *serverReconfigScenario
}

// Name returns the name of the feature.
func (f *serverReconfigFeature) Name() string {
	return f.name
}

// Setup initializes the feature test.
func (f *serverReconfigFeature) Setup() types.StepFunc {
	return f.scenario.Setup
}

// Run executes the server tier reconfiguration test.
//
// Every server, whatever tiers are enabled, gets the same single unified
// "klio" PVC (mounted at /klio). Adding tier2 to a tier1-only server
// therefore no longer touches the StatefulSet's VolumeClaimTemplates, so it
// should update the StatefulSet in place rather than delete/recreate it.
// This test verifies that:
//  1. The server Pod comes back ready with the new configuration.
//  2. The StatefulSet still has exactly one VolumeClaimTemplate, named "klio".
//  3. The klio PVC's UID is unchanged (it was not deleted/recreated) and no
//     new PVC was created for the server.
func (f *serverReconfigFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running server tier reconfiguration test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		server := f.scenario.klioServer
		stsName := server.Name + "-klio"
		klioPVCName := "klio-" + stsName + "-0"

		// Record the unified klio PVC's UID and the total PVC count for this
		// server before reconfiguration.
		originalPVC := &corev1.PersistentVolumeClaim{}
		require.NoError(t,
			r.Get(ctx, klioPVCName, f.scenario.namespace.Name, originalPVC),
			"failed to get original klio PVC %s", klioPVCName,
		)
		originalPVCUID := originalPVC.UID
		t.Logf("Recorded klio PVC %s with UID %s", klioPVCName, originalPVCUID)

		originalPVCCount := countServerPVCs(ctx, t, r, f.scenario.namespace.Name, server.Name)

		// Record the StatefulSet's reconcile-hash annotation before the
		// update. Adding tier2 no longer touches VolumeClaimTemplates, so
		// unlike before, the pod isn't forced to restart by an immutable
		// field rejection; without this gate, "wait for pod ready" could
		// trivially pass against the still-running pre-update pod before
		// the operator has reconciled anything.
		stsBefore := &appsv1.StatefulSet{}
		require.NoError(t,
			r.Get(ctx, stsName, f.scenario.namespace.Name, stsBefore),
			"failed to get StatefulSet before reconfiguration",
		)
		hashBefore := stsBefore.Annotations["klio.cnpg.io/klio-server-hash"]

		// Fetch current Server and add tier2 configuration
		currentServer := &kliov1alpha1.Server{}
		require.NoError(t,
			r.Get(ctx, server.Name, f.scenario.namespace.Name, currentServer),
			"failed to get current Server",
		)

		tier2Config := klio.BuildTier2Configuration(f.scenario.s3Opts, f.scenario.tier2Encryption)
		currentServer.Spec.Tier2 = &tier2Config
		require.NoError(t, r.Update(ctx, currentServer), "failed to update Server with tier2")
		t.Log("Server updated with tier2 configuration")

		// Wait for the operator to actually reconcile the tier2 change into
		// the StatefulSet before checking readiness/PVC identity.
		t.Log("Waiting for the StatefulSet to pick up the tier2 configuration...")
		err = wait.For(
			func(ctx context.Context) (bool, error) {
				sts := &appsv1.StatefulSet{}
				if getErr := r.Get(ctx, stsName, f.scenario.namespace.Name, sts); getErr != nil {
					return false, nil //nolint:nilerr
				}

				return sts.Annotations["klio.cnpg.io/klio-server-hash"] != hashBefore, nil
			},
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "StatefulSet was not reconciled with the tier2 configuration")

		// Wait for server Pod to become ready again
		t.Log("Waiting for server Pod to be ready after reconfiguration...")
		err = wait.For(
			conditions.KlioServerIsReady(r, server),
			wait.WithTimeout(10*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "server Pod not ready after tier2 reconfiguration")
		t.Log("Server Pod is ready after reconfiguration")

		// Verify the StatefulSet still has exactly one VolumeClaimTemplate:
		// the unified "klio" one, unaffected by the tier1/tier2 change.
		sts := &appsv1.StatefulSet{}
		require.NoError(t,
			r.Get(ctx, stsName, f.scenario.namespace.Name, sts),
			"failed to get StatefulSet",
		)
		require.Len(t, sts.Spec.VolumeClaimTemplates, 1,
			"StatefulSet should have exactly one VolumeClaimTemplate, got %d",
			len(sts.Spec.VolumeClaimTemplates))
		require.Equal(t, "klio", sts.Spec.VolumeClaimTemplates[0].Name,
			"StatefulSet's single VolumeClaimTemplate should be named %q", "klio")

		// Verify the klio PVC was retained (same UID), not deleted/recreated.
		finalPVC := &corev1.PersistentVolumeClaim{}
		require.NoError(t,
			r.Get(ctx, klioPVCName, f.scenario.namespace.Name, finalPVC),
			"klio PVC %s no longer exists after reconfiguration", klioPVCName,
		)
		require.Equal(t, originalPVCUID, finalPVC.UID,
			"klio PVC %s was recreated (UID changed from %s to %s), data may have been lost",
			klioPVCName, originalPVCUID, finalPVC.UID,
		)
		t.Logf("klio PVC %s retained with original UID %s", klioPVCName, finalPVC.UID)

		// Verify no new PVC appeared for this server.
		finalPVCCount := countServerPVCs(ctx, t, r, f.scenario.namespace.Name, server.Name)
		require.Equal(t, originalPVCCount, finalPVCCount,
			"number of PVCs for server %s changed after reconfiguration (%d -> %d)",
			server.Name, originalPVCCount, finalPVCCount,
		)

		t.Log("Server tier reconfiguration test passed: all verifications succeeded")

		return ctx
	}
}

// countServerPVCs returns the number of PersistentVolumeClaims labelled as
// belonging to the given Klio server, in the given namespace.
func countServerPVCs(
	ctx context.Context,
	t *testing.T,
	r *resources.Resources,
	namespace string,
	serverName string,
) int {
	t.Helper()

	var pvcList corev1.PersistentVolumeClaimList
	require.NoError(t,
		r.List(ctx, &pvcList, resources.WithLabelSelector("klio.cnpg.io/klio-server="+serverName)),
		"failed to list PVCs for server %s", serverName,
	)

	count := 0
	for i := range pvcList.Items {
		if pvcList.Items[i].Namespace == namespace {
			count++
		}
	}

	return count
}

// Teardown cleans up resources after the test.
func (f *serverReconfigFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

// ServerTierReconfiguration returns a Feature that tests adding tier2 to an existing tier1-only server.
func ServerTierReconfiguration(namespace string) *serverReconfigFeature {
	const (
		klioServerName        = "klio"
		selfSignedIssuerName  = "selfsigned-issuer"
		caCertificateName     = klioServerName + "-ca"
		caIssuerName          = caCertificateName + "-issuer"
		serverCertificateName = klioServerName + "-server"
		clientCertName        = "reconfig-client"
		rustfsName            = "rustfs"
		rustfsSecretName      = rustfsName + "-secret"
		rustfsConfigMapName   = rustfsName + "-config"
		encryptionSecretName  = "encryption"
		encryptionPassword    = "testencryptionpassword123"
		s3Prefix              = "tier2"
	)

	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject(selfSignedIssuerName, namespace)

	// RustFS infrastructure
	rustfsSecret := rustfs.GetRustFSSecret(rustfsSecretName, namespace)
	rustfsConfigMap := rustfs.GetRustFSConfigMap(rustfsConfigMapName, namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(rustfsName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(rustfsName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(rustfsName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		rustfsName, namespace, rustfs.RustFSBucketName)

	// Klio Server certificates and secrets
	caCertificate := certificates.GetCACertificateObject(caCertificateName, namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject(caIssuerName, namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject(
		serverCertificateName, namespace, []string{klioServerName}, issuer)
	userCertificate := certificates.GetUserCertificateObject(
		clientCertName, namespace, clientCertName+"@reconfig", caIssuer)
	ageSecrets := secrets.GetKlioAgeEncryptionSecrets(encryptionSecretName, namespace, encryptionPassword)

	// S3 options for tier2 (used during Run to add tier2)
	s3Opts := klio.Tier2S3Options{
		S3BucketName:          rustfs.RustFSBucketName,
		S3Prefix:              s3Prefix,
		S3Endpoint:            rustfs.GetRustFSEndpoint(rustfsName, namespace),
		S3Region:              rustfs.RustFSRegion,
		S3AccessKeySecretName: rustfsSecret.Name,
		S3SecretKeySecretName: rustfsSecret.Name,
		S3CABundleSecretName:  rustfsCertificate.Spec.SecretName,
	}

	// Create tier1-only server (no tier2)
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
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
	)

	scenario := &serverReconfigScenario{
		name:                  "ServerTierReconfiguration",
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
		s3Opts:                s3Opts,
		storageClass:          testCfg.StorageClass,
		tier2Encryption: klio.EncryptionOptions{
			EncryptionKeySecretName: ageSecrets.EncryptionKeySecret.Name,
			EncryptionKeyFileName:   "encryption-key.age",
			IdentitySecretName:      ageSecrets.IdentitySecret.Name,
			IdentityFileName:        "identity.txt",
		},
	}

	return &serverReconfigFeature{
		name:     "ServerTierReconfiguration",
		scenario: scenario,
	}
}

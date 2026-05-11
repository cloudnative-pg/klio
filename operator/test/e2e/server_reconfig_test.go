package e2e

import (
	"context"
	"slices"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
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
	rustfsPVC             *corev1.PersistentVolumeClaim
	rustfsLogsPVC         *corev1.PersistentVolumeClaim
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

	// Klio Server (initially tier1+queue only, no tier2)
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
// This test verifies that adding tier2 to an existing tier1+queue server
// triggers the StatefulSet delete/recreate flow (due to immutable VCTs)
// and that:
//  1. The server Pod comes back ready with the new configuration.
//  2. The StatefulSet has the expected VolumeClaimTemplates including cachetier2.
//  3. The new cachetier2 PVC is created.
//  4. The original PVCs (data, cachetier1, queue) are retained (same UIDs).
func (f *serverReconfigFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()
		t.Log("Running server tier reconfiguration test")

		r, err := resources.New(cfg.Client().RESTConfig())
		require.NoError(t, err, "failed to create resources client")

		server := f.scenario.klioServer
		stsName := server.Name + "-klio"

		// Record UIDs of existing PVCs before reconfiguration
		originalPVCNames := []string{
			"data-" + stsName + "-0",
			"cachetier1-" + stsName + "-0",
			"queue-" + stsName + "-0",
		}
		originalPVCUIDs := make(map[string]k8stypes.UID, len(originalPVCNames))

		for _, pvcName := range originalPVCNames {
			pvc := &corev1.PersistentVolumeClaim{}
			require.NoError(t,
				r.Get(ctx, pvcName, f.scenario.namespace.Name, pvc),
				"failed to get original PVC %s", pvcName,
			)
			originalPVCUIDs[pvcName] = pvc.UID
			t.Logf("Recorded PVC %s with UID %s", pvcName, pvc.UID)
		}

		// Fetch current Server and add tier2 configuration
		currentServer := &kliov1alpha1.Server{}
		require.NoError(t,
			r.Get(ctx, server.Name, f.scenario.namespace.Name, currentServer),
			"failed to get current Server",
		)

		tier2Config := klio.BuildTier2Configuration(
			f.scenario.s3Opts, f.scenario.tier2Encryption, f.scenario.storageClass)
		currentServer.Spec.Tier2 = &tier2Config
		require.NoError(t, r.Update(ctx, currentServer), "failed to update Server with tier2")
		t.Log("Server updated with tier2 configuration")

		// Wait for server Pod to become ready again
		t.Log("Waiting for server Pod to be ready after reconfiguration...")
		err = wait.For(
			conditions.KlioServerIsReady(r, server),
			wait.WithTimeout(10*time.Minute),
			wait.WithInterval(10*time.Second),
		)
		require.NoError(t, err, "server Pod not ready after tier2 reconfiguration")
		t.Log("Server Pod is ready after reconfiguration")

		// Verify StatefulSet has the expected VolumeClaimTemplates
		sts := &appsv1.StatefulSet{}
		require.NoError(t,
			r.Get(ctx, stsName, f.scenario.namespace.Name, sts),
			"failed to get StatefulSet",
		)

		vctNames := make([]string, len(sts.Spec.VolumeClaimTemplates))
		for i, vct := range sts.Spec.VolumeClaimTemplates {
			vctNames[i] = vct.Name
		}
		t.Logf("StatefulSet VolumeClaimTemplates: %v", vctNames)

		for _, expected := range []string{"data", "cachetier1", "queue", "cachetier2"} {
			require.True(t,
				slices.Contains(vctNames, expected),
				"StatefulSet missing VolumeClaimTemplate %q, got %v", expected, vctNames,
			)
		}

		// Verify cachetier2 PVC exists
		cachetier2PVC := &corev1.PersistentVolumeClaim{}
		cachetier2PVCName := "cachetier2-" + stsName + "-0"
		require.NoError(t,
			r.Get(ctx, cachetier2PVCName, f.scenario.namespace.Name, cachetier2PVC),
			"cachetier2 PVC %s not found", cachetier2PVCName,
		)
		t.Logf("cachetier2 PVC %s exists with UID %s", cachetier2PVCName, cachetier2PVC.UID)

		// Verify original PVCs are retained (same UIDs)
		for _, pvcName := range originalPVCNames {
			pvc := &corev1.PersistentVolumeClaim{}
			require.NoError(t,
				r.Get(ctx, pvcName, f.scenario.namespace.Name, pvc),
				"original PVC %s no longer exists after reconfiguration", pvcName,
			)
			require.Equal(t, originalPVCUIDs[pvcName], pvc.UID,
				"PVC %s was recreated (UID changed from %s to %s), data may have been lost",
				pvcName, originalPVCUIDs[pvcName], pvc.UID,
			)
			t.Logf("PVC %s retained with original UID %s", pvcName, pvc.UID)
		}

		t.Log("Server tier reconfiguration test passed: all verifications succeeded")

		return ctx
	}
}

// Teardown cleans up resources after the test.
func (f *serverReconfigFeature) Teardown() types.StepFunc {
	return f.scenario.Teardown
}

// ServerTierReconfiguration returns a Feature that tests adding tier2 to an existing tier1+queue server.
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
		rustfsDataPVCName     = rustfsName + "-data"
		rustfsLogsPVCName     = rustfsName + "-logs"
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
	rustfsPVC := rustfs.GetRustFSPVC(rustfsDataPVCName, namespace)
	rustfsLogsPVC := rustfs.GetRustFSLogsPVC(rustfsLogsPVCName, namespace)
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

	// Create tier1+queue only server (no tier2)
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
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
	)

	scenario := &serverReconfigScenario{
		name:                  "ServerTierReconfiguration",
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

package e2e

import (
	"context"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
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
	t.Logf("NOTE: Second Klio Server will be created after backup reaches tier2 (during recovery setup)")

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

// deployRecoveryServer creates the second Klio Server after tier2 replication.
func (s *tier2RecoveryScenario) deployRecoveryServer(
	ctx context.Context,
	_ *cnpgv1.Cluster,
	r *resources.Resources,
) error {
	return deployTier2RecoveryServer(ctx, r, s.namespace.Name, s.klioServer.Name, &tier2RecoveryServerResources{
		RecoveryServerCertificate:   s.recoveryServerCertificate,
		RecoveryServerCACertificate: s.recoveryServerCACertificate,
		RecoveryServerCAIssuer:      s.recoveryServerCAIssuer,
		RecoveryUserCertificate:     s.recoveryUserCertificate,
		RecoveryServer:              s.recoveryServer,
		PluginConfigurationRecovery: s.klioPluginConfigurationRecovery,
	})
}

// NewTier2RecoveryFeatureConfig creates a new tier2 recovery feature configuration.
func NewTier2RecoveryFeatureConfig(
	name string, instances int, namespace string,
) machineryFeatures.RecoveryFeatureConfig {
	// Build all resources using the shared builder
	res := buildTier2ScenarioResources(namespace, instances)

	// Create scenario from shared resources
	scenario := &tier2RecoveryScenario{
		namespace:                       res.Namespace,
		issuer:                          res.Issuer,
		rustfsSecret:                    res.RustfsSecret,
		rustfsConfigMap:                 res.RustfsConfigMap,
		rustfsPVC:                       res.RustfsPVC,
		rustfsLogsPVC:                   res.RustfsLogsPVC,
		rustfsCertificate:               res.RustfsCertificate,
		rustfsService:                   res.RustfsService,
		rustfsDeployment:                res.RustfsDeployment,
		rustfsCreateBucketJob:           res.RustfsCreateBucketJob,
		serverCertificate:               res.ServerCertificate,
		caCertificate:                   res.CACertificate,
		caIssuer:                        res.CAIssuer,
		userCertificate:                 res.UserCertificate,
		encryptionSecret:                res.EncryptionSecret,
		klioServer:                      res.KlioServer,
		cnpgCluster:                     res.CNPGCluster,
		klioPluginConfigurationSource:   res.KlioPluginConfigurationSource,
		recoveryServerCertificate:       res.RecoveryServerCertificate,
		recoveryServerCACertificate:     res.RecoveryServerCACertificate,
		recoveryServerCAIssuer:          res.RecoveryServerCAIssuer,
		recoveryUserCertificate:         res.RecoveryUserCertificate,
		recoveryServer:                  res.RecoveryServer,
		klioPluginConfigurationRecovery: res.KlioPluginConfigurationRecovery,
		name:                            name,
	}

	recoveryConfig := machineryFeatures.RecoveryFeatureConfig{
		Name:                  name,
		Setup:                 scenario.Setup,
		Teardown:              scenario.Teardown,
		SourcePrimaryPod:      &scenario.sourcePrimaryPod,
		Backup:                res.Backup,
		RecoveryCluster:       res.RecoveryCluster,
		MutateRecoveryCluster: []machineryFeatures.RecoveryClusterMutateFunc{scenario.deployRecoveryServer},
	}

	return recoveryConfig
}

// RecoverClusterFromTier2 returns a RecoveryFeature for tier2 recovery testing.
func RecoverClusterFromTier2(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(
		NewTier2RecoveryFeatureConfig("RecoverClusterFromTier2", 1, namespace))
}

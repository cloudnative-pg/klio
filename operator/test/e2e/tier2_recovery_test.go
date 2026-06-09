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
	"github.com/cloudnative-pg/klio/operator/test/klio/infra"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
)

// tier2RecoveryScenario contains all resources needed for tier2 recovery testing.
type tier2RecoveryScenario struct {
	// Common
	namespace *corev1.Namespace
	issuer    *certmanagerv1.Issuer

	// RustFS infrastructure
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
	createNamespace(ctx, t, r, s.namespace)

	// Set scenario infra
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
	require.NoError(t, r.Create(ctx, s.klioPluginConfigurationSource),
		"failed to create Klio plugin configuration for source cluster")
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
		rustfsCertificate:               res.RustfsCertificate,
		rustfsService:                   res.RustfsService,
		rustfsDeployment:                res.RustfsDeployment,
		rustfsCreateBucketJob:           res.RustfsCreateBucketJob,
		serverCertificate:               res.ServerCertificate,
		caCertificate:                   res.CACertificate,
		caIssuer:                        res.CAIssuer,
		userCertificate:                 res.UserCertificate,
		encryptionSecret:                res.EncryptionSecret,
		identitySecret:                  res.IdentitySecret,
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
		BackupTimeout:         5 * time.Minute,
	}

	return recoveryConfig
}

// RecoverClusterFromTier2 returns a RecoveryFeature for tier2 recovery testing.
func RecoverClusterFromTier2(namespace string) *machineryFeatures.RecoveryFeature {
	return machineryFeatures.NewRecoveryFeature(
		NewTier2RecoveryFeatureConfig("RecoverClusterFromTier2", 1, namespace))
}

package e2e

import (
	"context"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
)

const (
	klioServerName = "test-klio-server"
)

type commonBackupRestoreScenario struct {
	namespace                       *corev1.Namespace
	userCertificate                 *certmanagerv1.Certificate
	encryptionSecret                *corev1.Secret
	cnpgCluster                     *cnpgv1.Cluster
	issuer                          *certmanagerv1.Issuer
	certificate                     *certmanagerv1.Certificate
	caIssuer                        *certmanagerv1.Issuer
	caCertificate                   *certmanagerv1.Certificate
	klioServer                      *kliov1alpha1.Server
	klioPluginConfigurationSource   *kliov1alpha1.PluginConfiguration
	klioPluginConfigurationRecovery *kliov1alpha1.PluginConfiguration

	name string

	sourcePrimaryPod corev1.Pod
}

func (c *commonBackupRestoreScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for recovery feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Create(ctx, c.namespace), "failed to create namespace")
	require.NoError(t, r.Create(ctx, c.cnpgCluster), "failed to create CNPG source Cluster")
	require.NoError(t, r.Create(ctx, c.klioPluginConfigurationSource),
		"failed to create Klio plugin configuration for source cluster")
	require.NoError(t, r.Create(ctx, c.userCertificate), "failed to create user secret")
	require.NoError(t, r.Create(ctx, c.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, c.issuer), "failed to create issuer")
	require.NoError(t, r.Create(ctx, c.caIssuer), "failed to create CA issuer")
	require.NoError(t, r.Create(ctx, c.caCertificate), "failed to create CA certificate")
	require.NoError(t, r.Create(ctx, c.certificate), "failed to create certificate")
	require.NoError(t, r.Create(ctx, c.klioServer), "failed to create KLIO server")

	// Not all tests require a recovery cluster, so we allow for it to be nil.
	if c.klioPluginConfigurationRecovery != nil {
		require.NoError(t, r.Create(ctx, c.klioPluginConfigurationRecovery),
			"failed to create Klio plugin configuration for recovery cluster")
	}

	t.Logf("Waiting for resources to be ready for recovery feature: %s", c.name)
	err = wait.For(
		machineryConditions.ClusterIsReady(r, c.cnpgCluster),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for CNPG source Cluster to be ready")

	err = wait.For(
		conditions.KlioServerIsReady(r, c.klioServer),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for Klio server to be ready")

	require.NoError(
		t,
		r.Get(
			ctx,
			c.cnpgCluster.Status.CurrentPrimary,
			c.namespace.Name,
			&c.sourcePrimaryPod,
		),
		"failed to get the current primary pod",
	)

	t.Logf("Resources created and ready for recovery feature: %s", c.name)

	return ctx
}

func (c *commonBackupRestoreScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for recovery feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Delete(ctx, c.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for recovery feature: %s", c.name)

	return ctx
}

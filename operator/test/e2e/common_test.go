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

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

const klioServerName = "test-klio-server"

// imagePullSecretName is the fixed name of the pull secret created in each
// test namespace when registry credentials are configured.
const imagePullSecretName = "e2e-pull-secret" //nolint:gosec // This is a fixed name for the pull secret used in tests.

// pullSecretName returns the pull secret name when registry credentials are
// configured, or an empty string when they are not.
func pullSecretName() string {
	if testCfg.ImagePullSecret.IsConfigured() {
		return imagePullSecretName
	}

	return ""
}

// createNamespace creates the namespace and, when registry credentials are
// configured in testCfg, creates a dockerconfigjson pull secret inside it.
func createNamespace(ctx context.Context, t *testing.T, r *resources.Resources, ns *corev1.Namespace) {
	t.Helper()
	require.NoError(t, r.Create(ctx, ns), "failed to create namespace")
	if !testCfg.ImagePullSecret.IsConfigured() {
		return
	}
	cfg := testCfg.ImagePullSecret
	secret := secrets.GetDockerConfigJSONSecret(
		imagePullSecretName, ns.Name, cfg.Registry, cfg.Username, cfg.Password)
	require.NoError(t, r.Create(ctx, secret), "failed to create pull secret")
}

// pluginTestResources holds the common Kubernetes resources needed by
// plugin-level e2e tests (PluginConfiguration update, missing PC, etc.).
type pluginTestResources struct {
	namespace               *corev1.Namespace
	issuer                  *certmanagerv1.Issuer
	caIssuer                *certmanagerv1.Issuer
	caCertificate           *certmanagerv1.Certificate
	certificate             *certmanagerv1.Certificate
	userCertificate         *certmanagerv1.Certificate
	encryptionSecret        *corev1.Secret
	identitySecret          *corev1.Secret
	klioServer              *kliov1alpha1.Server
	cnpgCluster             *cnpgv1.Cluster
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
}

// newPluginTestResources creates the standard set of resources for a
// single-instance cluster with a Klio plugin configuration.
func newPluginTestResources(namespace string) pluginTestResources {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	userCertificate := certificates.GetUserCertificateObject(
		"klio-user", namespace, "klio-user@test-cluster", caIssuer)
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: userCertificate,
			ClusterName:       "test-cluster",
		},
	)

	cnpgCluster := cnpg.GetCnpgClusterObject("test-cluster", namespace, 1, "klio-plugin-configuration",
		cnpg.ClusterTemplateOptions{ImagePullSecret: pullSecretName(), StorageClass: testCfg.StorageClass})

	ageSecrets := secrets.GetKlioAgeEncryptionSecrets("encryption", namespace, "testencryptionpassword123")
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			Image:              testCfg.ServerImage,
			StorageClass:       testCfg.StorageClass,
			ImagePullSecret:    pullSecretName(),
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

	return pluginTestResources{
		namespace:               namespaceObj,
		issuer:                  issuer,
		caIssuer:                caIssuer,
		caCertificate:           caCertificate,
		certificate:             certificate,
		userCertificate:         userCertificate,
		encryptionSecret:        ageSecrets.EncryptionKeySecret,
		identitySecret:          ageSecrets.IdentitySecret,
		klioServer:              klioServer,
		cnpgCluster:             cnpgCluster,
		klioPluginConfiguration: klioPluginConfiguration,
	}
}

type commonBackupRestoreScenario struct {
	namespace                       *corev1.Namespace
	userCertificate                 *certmanagerv1.Certificate
	encryptionSecret                *corev1.Secret
	identitySecret                  *corev1.Secret
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
	createNamespace(ctx, t, r, c.namespace)
	require.NoError(t, r.Create(ctx, c.cnpgCluster), "failed to create CNPG source Cluster")
	require.NoError(t, r.Create(ctx, c.klioPluginConfigurationSource),
		"failed to create Klio plugin configuration for source cluster")
	require.NoError(t, r.Create(ctx, c.userCertificate), "failed to create user secret")
	require.NoError(t, r.Create(ctx, c.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, c.identitySecret), "failed to create identity secret")
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
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for CNPG source Cluster to be ready")

	err = wait.For(
		conditions.KlioServerIsReady(r, c.klioServer),
		wait.WithTimeout(4*time.Minute),
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

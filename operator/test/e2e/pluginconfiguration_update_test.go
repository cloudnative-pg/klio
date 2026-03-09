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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/cnpgi"
	klioConditions "github.com/cloudnative-pg/klio/operator/test/klio/conditions"
	klioFeatures "github.com/cloudnative-pg/klio/operator/test/klio/features"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

type pluginConfigurationUpdateScenario struct {
	namespace               *corev1.Namespace
	issuer                  *certmanagerv1.Issuer
	caIssuer                *certmanagerv1.Issuer
	caCertificate           *certmanagerv1.Certificate
	certificate             *certmanagerv1.Certificate
	userCertificate         *certmanagerv1.Certificate
	encryptionSecret        *corev1.Secret
	klioServer              *kliov1alpha1.Server
	cnpgCluster             *cnpgv1.Cluster
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration

	name string
}

func (c *pluginConfigurationUpdateScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for PluginConfiguration update feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create all resources
	require.NoError(t, r.Create(ctx, c.namespace), "failed to create namespace")
	require.NoError(t, r.Create(ctx, c.issuer), "failed to create issuer")
	require.NoError(t, r.Create(ctx, c.caCertificate), "failed to create CA certificate")
	require.NoError(t, r.Create(ctx, c.caIssuer), "failed to create CA issuer")
	require.NoError(t, r.Create(ctx, c.certificate), "failed to create certificate")
	require.NoError(t, r.Create(ctx, c.userCertificate), "failed to create user certificate")
	require.NoError(t, r.Create(ctx, c.encryptionSecret), "failed to create encryption secret")
	require.NoError(t, r.Create(ctx, c.klioServer), "failed to create Klio server")
	require.NoError(t, r.Create(ctx, c.klioPluginConfiguration),
		"failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, c.cnpgCluster), "failed to create CNPG Cluster")

	t.Logf("Waiting for resources to be ready for PluginConfiguration update feature: %s", c.name)

	// Wait for Klio server to be ready
	err = wait.For(
		conditions.KlioServerIsReady(r, c.klioServer),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for Klio server to be ready")

	// Wait for CNPG cluster to be ready
	err = wait.For(
		machineryConditions.ClusterIsReady(r, c.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for CNPG Cluster to be ready")

	t.Logf("Resources created and ready for PluginConfiguration update feature: %s", c.name)

	return ctx
}

func (c *pluginConfigurationUpdateScenario) Run(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Running PluginConfiguration update test: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Step 1: Capture initial state
	t.Log("Capturing initial pod and PluginConfiguration state")

	var initialPod corev1.Pod
	require.NoError(t,
		r.Get(ctx, c.cnpgCluster.Status.CurrentPrimary, c.namespace.Name, &initialPod),
		"failed to get primary pod")

	// Record initial restart count for klio-plugin
	var initialRestartCount int32
	containerFound := false
	for _, containerStatus := range initialPod.Status.InitContainerStatuses {
		if containerStatus.Name == cnpgi.KlioPluginContainerName {
			initialRestartCount = containerStatus.RestartCount
			t.Logf("Initial restart count for klio-plugin: %d", containerStatus.RestartCount)
			containerFound = true

			break
		}
	}
	require.True(t, containerFound, "klio-plugin init container not found in pod")

	// Get current PluginConfiguration
	var currentPC kliov1alpha1.PluginConfiguration
	require.NoError(t,
		r.Get(ctx, c.klioPluginConfiguration.Name, c.namespace.Name, &currentPC),
		"failed to get PluginConfiguration")

	t.Logf("Initial PluginConfiguration generation: %d", currentPC.Generation)

	// Step 2: Update PluginConfiguration
	t.Log("Updating PluginConfiguration retention policy")

	if currentPC.Spec.Tier1 == nil {
		currentPC.Spec.Tier1 = &kliov1alpha1.Tier1PluginConfiguration{}
	}
	if currentPC.Spec.Tier1.RetentionPolicy == nil {
		currentPC.Spec.Tier1.RetentionPolicy = &kliov1alpha1.RetentionPolicy{}
	}
	currentPC.Spec.Tier1.RetentionPolicy.KeepLatest = ptr.To(5)

	require.NoError(t, r.Update(ctx, &currentPC), "failed to update PluginConfiguration")

	t.Logf("Updated PluginConfiguration, new generation: %d", currentPC.Generation)

	// Step 3: Wait for ConfigurationApplied condition
	t.Log("Waiting for ConfigurationApplied condition to be set")

	err = wait.For(
		klioConditions.PluginConfigurationHasCondition(
			r,
			c.klioPluginConfiguration,
			kliov1alpha1.PluginConfigurationConditionConfigurationApplied,
			metav1.ConditionTrue,
			currentPC.Generation,
		),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "ConfigurationApplied condition not set correctly")

	t.Log("ConfigurationApplied condition verified")

	// Step 4: Wait for init container restart
	t.Log("Waiting for init container to restart")

	err = wait.For(
		machineryConditions.InitContainerHasRestarted(
			r,
			c.cnpgCluster.Status.CurrentPrimary,
			c.namespace.Name,
			cnpgi.KlioPluginContainerName,
			initialRestartCount,
		),
		wait.WithTimeout(3*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "init container did not restart")

	// Verify final restart count
	var finalPod corev1.Pod
	require.NoError(t,
		r.Get(ctx, c.cnpgCluster.Status.CurrentPrimary, c.namespace.Name, &finalPod),
		"failed to get final pod state")

	for _, containerStatus := range finalPod.Status.InitContainerStatuses {
		if containerStatus.Name == cnpgi.KlioPluginContainerName {
			t.Logf("Final restart count for klio-plugin: %d (initial: %d)",
				containerStatus.RestartCount,
				initialRestartCount)

			break
		}
	}

	t.Log("Init container successfully restarted")

	return ctx
}

func (c *pluginConfigurationUpdateScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for PluginConfiguration update feature: %s", c.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")
	require.NoError(t, r.Delete(ctx, c.namespace), "failed to delete namespace")
	t.Logf("Resources torn down for PluginConfiguration update feature: %s", c.name)

	return ctx
}

// PluginConfigurationUpdate creates a feature that tests PluginConfiguration updates
// and verifies that sidecar containers restart when the configuration changes.
func PluginConfigurationUpdate(namespace string) *klioFeatures.PluginConfigurationUpdateFeature {
	// Create all resource objects
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)
	certificate := certificates.GetCertificateObject("test", namespace, []string{klioServerName}, issuer)

	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)

	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace, "klio-user@test-cluster", caIssuer)
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: certificate,
			ClientCertificate: userCertificate,
			ClusterName:       "test-cluster",
		},
	)

	cnpgCluster := cnpg.GetCnpgClusterObject("test-cluster", namespace, 1, "klio-plugin-configuration")

	encryptionSecret := secrets.GetKlioEncryptionSecret("encryption", namespace, "testencryptionpassword123")
	klioServer := klio.GetServerObject(
		klioServerName,
		namespace,
		klio.ServerTemplateOptions{
			TLSSecretName:        certificate.Spec.SecretName,
			ClientCASecretName:   caCertificate.Spec.SecretName,
			EncryptionSecretName: encryptionSecret.Name,
		},
	)

	scenario := &pluginConfigurationUpdateScenario{
		namespace:               namespaceObj,
		issuer:                  issuer,
		caIssuer:                caIssuer,
		caCertificate:           caCertificate,
		certificate:             certificate,
		userCertificate:         userCertificate,
		encryptionSecret:        encryptionSecret,
		klioServer:              klioServer,
		cnpgCluster:             cnpgCluster,
		klioPluginConfiguration: klioPluginConfiguration,
		name:                    "PluginConfigurationUpdate",
	}

	return klioFeatures.NewPluginConfigurationUpdateFeature(
		klioFeatures.PluginConfigurationUpdateFeatureConfig{
			Name:     "PluginConfigurationUpdate",
			Setup:    scenario.Setup,
			Run:      scenario.Run,
			Teardown: scenario.Teardown,
		},
	)
}

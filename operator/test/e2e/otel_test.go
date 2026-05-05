package e2e

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cnpgv1 "github.com/cloudnative-pg/api/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	machineryFeatures "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/features"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/pods"
	"github.com/cloudnative-pg/klio/operator/test/utils/conditions"
	"github.com/cloudnative-pg/klio/operator/test/utils/metrics"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/certificates"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/cnpg"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/klio"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/otel"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/rustfs"
	"github.com/cloudnative-pg/klio/operator/test/utils/templates/secrets"
)

// otelMetricsScenario holds the resources for the OTEL metrics e2e test.
type otelMetricsScenario struct {
	namespace *corev1.Namespace
	name      string

	// Common issuer
	issuer *certmanagerv1.Issuer

	// RustFS infrastructure
	rustfsSecret          *corev1.Secret
	rustfsConfigMap       *corev1.ConfigMap
	rustfsPVC             *corev1.PersistentVolumeClaim
	rustfsLogsPVC         *corev1.PersistentVolumeClaim
	rustfsCertificate     *certmanagerv1.Certificate
	rustfsService         *corev1.Service
	rustfsDeployment      *appsv1.Deployment
	rustfsCreateBucketJob *batchv1.Job

	// Certificates
	caCertificate         *certmanagerv1.Certificate
	caIssuer              *certmanagerv1.Issuer
	serverCertificate     *certmanagerv1.Certificate
	userCertificate       *certmanagerv1.Certificate
	otelCollectorCert     *certmanagerv1.Certificate
	otelServerClientCert  *certmanagerv1.Certificate
	otelClusterClientCert *certmanagerv1.Certificate

	// OTEL Collector resources
	otelServiceAccount     *corev1.ServiceAccount
	otelClusterRole        *rbacv1.ClusterRole
	otelClusterRoleBinding *rbacv1.ClusterRoleBinding
	otelConfigMap          *corev1.ConfigMap
	otelDeployment         *appsv1.Deployment
	otelService            *corev1.Service

	// Klio and CNPG resources
	encryptionSecrets       *secrets.AgeEncryptionSecrets
	klioServer              *kliov1alpha1.Server
	klioServerOTELConfig    *corev1.ConfigMap
	klioPluginConfiguration *kliov1alpha1.PluginConfiguration
	cnpgCluster             *cnpgv1.Cluster

	// Backup
	backup *cnpgv1.Backup
}

//nolint:funlen
func newOTELMetricsScenario(namespace string) *otelMetricsScenario {
	const (
		rustfsName = "rustfs"
		s3Prefix   = "tier2"
	)

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}

	// Common issuer
	issuer := certificates.GetSelfSignedIssuerObject("selfsigned-issuer", namespace)

	// RustFS infrastructure
	rustfsSecret := rustfs.GetRustFSSecret(rustfsName+"-secret", namespace)
	rustfsConfigMap := rustfs.GetRustFSConfigMap(rustfsName+"-config", namespace)
	rustfsPVC := rustfs.GetRustFSPVC(rustfsName+"-data", namespace)
	rustfsLogsPVC := rustfs.GetRustFSLogsPVC(rustfsName+"-logs", namespace)
	rustfsCertificate := rustfs.GetRustFSCertificate(rustfsName, namespace, issuer)
	rustfsService := rustfs.GetRustFSService(rustfsName, namespace)
	rustfsDeployment := rustfs.GetRustFSDeployment(rustfsName, namespace)
	rustfsCreateBucketJob := rustfs.GetRustFSCreateBucketJob(
		rustfsName, namespace, rustfs.RustFSBucketName)

	// Certificates setup
	caCertificate := certificates.GetCACertificateObject("test-ca", namespace, issuer)
	caIssuer := certificates.GetCAIssuerObject("test-ca-issuer", namespace, caCertificate.Spec.SecretName)
	serverCertificate := certificates.GetCertificateObject("klio-server", namespace, []string{klioServerName}, issuer)
	userCertificate := certificates.GetUserCertificateObject("klio-user", namespace, "klio-user@test-cluster", caIssuer)

	// OTEL Collector certificates
	otelCollectorCert := otel.GetCollectorCertificate(namespace, issuer)
	otelServerClientCert := otel.GetOTELClientCertificate("klio-server-otel-client", namespace, issuer)
	otelClusterClientCert := otel.GetOTELClientCertificate("cluster-otel-client", namespace, issuer)

	// OTEL Collector resources
	otelServiceAccount := otel.GetCollectorServiceAccount(namespace)
	otelClusterRole := otel.GetCollectorClusterRole(namespace)
	otelClusterRoleBinding := otel.GetCollectorClusterRoleBinding(namespace)
	otelConfigMap := otel.GetCollectorConfigMap(namespace)
	otelDeployment := otel.GetCollectorDeployment(namespace)
	otelService := otel.GetCollectorService(namespace)

	// Klio server with OTEL configuration and Tier 2
	encryptionSecrets := secrets.GetKlioAgeEncryptionSecrets("encryption", namespace, "testencryptionpassword123")
	klioServerOTELConfig := otel.GetKlioServerOTELConfigMap(namespace)

	encOpts := klio.EncryptionOptions{
		EncryptionKeySecretName: encryptionSecrets.EncryptionKeySecret.Name,
		EncryptionKeyFileName:   "encryption-key.age",
		IdentitySecretName:      encryptionSecrets.IdentitySecret.Name,
		IdentityFileName:        "identity.txt",
	}

	klioServer := klio.GetServerWithTier2Object(
		klioServerName,
		namespace,
		klio.ServerWithTier2TemplateOptions{
			ServerTemplateOptions: klio.ServerTemplateOptions{
				TLSSecretName:      serverCertificate.Spec.SecretName,
				ClientCASecretName: caCertificate.Spec.SecretName,
				Encryption:         encOpts,
			},
			Tier2Encryption: encOpts,
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

	// Add OTEL configuration to Klio server
	klioServer.Spec.Template = &kliov1alpha1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "server",
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: klioServerOTELConfig.Name},
						}},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "otel-certs", MountPath: "/otel", ReadOnly: true},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "otel-certs",
					VolumeSource: corev1.VolumeSource{
						Projected: &corev1.ProjectedVolumeSource{
							Sources: []corev1.VolumeProjection{
								{
									Secret: &corev1.SecretProjection{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: otelCollectorCert.Spec.SecretName,
										},
										Items: []corev1.KeyToPath{
											{Key: "ca.crt", Path: "ca.crt"},
										},
									},
								},
								{
									Secret: &corev1.SecretProjection{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: otelServerClientCert.Spec.SecretName,
										},
										Items: []corev1.KeyToPath{
											{Key: "tls.crt", Path: "tls.crt"},
											{Key: "tls.key", Path: "tls.key"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Plugin configuration with Tier 2 backup enabled
	klioPluginConfiguration := klio.GetPluginConfigurationObject(
		"klio-plugin-configuration",
		namespace,
		klio.PluginConfigurationTemplateOptions{
			ServerCertificate: serverCertificate,
			ClientCertificate: userCertificate,
			ClusterName:       "test-cluster",
			EnableTier2Backup: true,
		},
	)

	// CNPG Cluster with OTEL configuration
	cnpgCluster := cnpg.GetCnpgClusterObject("test-cluster", namespace, 1, klioPluginConfiguration.Name)

	// Add OTEL env vars as Spec.Env (not EnvFrom) so the Klio lifecycle webhook
	// merges them into the sidecar container.
	cnpgCluster.Spec.Env = otel.GetClusterOTELEnvVars()
	cnpgCluster.Spec.ProjectedVolumeTemplate = &corev1.ProjectedVolumeSource{
		Sources: []corev1.VolumeProjection{
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: otelCollectorCert.Spec.SecretName,
					},
					Items: []corev1.KeyToPath{
						{Key: "ca.crt", Path: "otel-ca.crt"},
					},
				},
			},
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: otelClusterClientCert.Spec.SecretName,
					},
					Items: []corev1.KeyToPath{
						{Key: "tls.crt", Path: "otel-tls.crt"},
						{Key: "tls.key", Path: "otel-tls.key"},
					},
				},
			},
		},
	}

	// Backup
	backup := cnpg.GetCnpgBackupObject("test-backup", namespace, cnpgv1.BackupTargetPrimary, cnpgCluster)

	return &otelMetricsScenario{
		namespace:               ns,
		name:                    "OTELMetricsAndTraces",
		issuer:                  issuer,
		rustfsSecret:            rustfsSecret,
		rustfsConfigMap:         rustfsConfigMap,
		rustfsPVC:               rustfsPVC,
		rustfsLogsPVC:           rustfsLogsPVC,
		rustfsCertificate:       rustfsCertificate,
		rustfsService:           rustfsService,
		rustfsDeployment:        rustfsDeployment,
		rustfsCreateBucketJob:   rustfsCreateBucketJob,
		caCertificate:           caCertificate,
		caIssuer:                caIssuer,
		serverCertificate:       serverCertificate,
		userCertificate:         userCertificate,
		otelCollectorCert:       otelCollectorCert,
		otelServerClientCert:    otelServerClientCert,
		otelClusterClientCert:   otelClusterClientCert,
		otelServiceAccount:      otelServiceAccount,
		otelClusterRole:         otelClusterRole,
		otelClusterRoleBinding:  otelClusterRoleBinding,
		otelConfigMap:           otelConfigMap,
		otelDeployment:          otelDeployment,
		otelService:             otelService,
		encryptionSecrets:       encryptionSecrets,
		klioServer:              klioServer,
		klioServerOTELConfig:    klioServerOTELConfig,
		klioPluginConfiguration: klioPluginConfiguration,
		cnpgCluster:             cnpgCluster,
		backup:                  backup,
	}
}

//nolint:funlen
func (s *otelMetricsScenario) Setup(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Creating resources for OTEL metrics test: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Create namespace
	require.NoError(t, r.Create(ctx, s.namespace), "failed to create namespace")

	// Create resources that have no dependencies
	require.NoError(t, r.Create(ctx, s.otelServiceAccount), "failed to create OTEL service account")
	require.NoError(t, r.Create(ctx, s.otelClusterRole), "failed to create OTEL cluster role")
	require.NoError(t, r.Create(ctx, s.otelClusterRoleBinding), "failed to create OTEL cluster role binding")
	require.NoError(t, r.Create(ctx, s.otelConfigMap), "failed to create OTEL config map")
	require.NoError(t, r.Create(ctx, s.klioServerOTELConfig), "failed to create Klio server OTEL config")
	require.NoError(t, r.Create(ctx, s.encryptionSecrets.EncryptionKeySecret), "failed to create encryption key secret")
	require.NoError(t, r.Create(ctx, s.encryptionSecrets.IdentitySecret), "failed to create identity secret")
	require.NoError(t, r.Create(ctx, s.rustfsSecret), "failed to create RustFS secret")
	require.NoError(t, r.Create(ctx, s.rustfsConfigMap), "failed to create RustFS configmap")
	require.NoError(t, r.Create(ctx, s.rustfsPVC), "failed to create RustFS data PVC")
	require.NoError(t, r.Create(ctx, s.rustfsLogsPVC), "failed to create RustFS logs PVC")

	// Create and wait for the self-signed issuer
	require.NoError(t, r.Create(ctx, s.issuer), "failed to create issuer")
	err = wait.For(
		conditions.IssuerIsReady(r, s.issuer),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "issuer not ready")

	// Create all certificates that depend on the self-signed issuer
	require.NoError(t, r.Create(ctx, s.caCertificate), "failed to create CA certificate")
	require.NoError(t, r.Create(ctx, s.serverCertificate), "failed to create server certificate")
	require.NoError(t, r.Create(ctx, s.rustfsCertificate), "failed to create RustFS certificate")
	require.NoError(t, r.Create(ctx, s.otelCollectorCert), "failed to create OTEL collector certificate")
	require.NoError(t, r.Create(ctx, s.otelServerClientCert), "failed to create OTEL server client certificate")
	require.NoError(t, r.Create(ctx, s.otelClusterClientCert), "failed to create OTEL cluster client certificate")

	// Wait for the CA certificate so we can create the CA issuer
	err = wait.For(
		conditions.CertificateIsReady(r, s.caCertificate),
		wait.WithTimeout(1*time.Minute),
		wait.WithInterval(5*time.Second),
	)
	require.NoError(t, err, "CA certificate not ready")

	require.NoError(t, r.Create(ctx, s.caIssuer), "failed to create CA issuer")
	require.NoError(t, r.Create(ctx, s.userCertificate), "failed to create user certificate")

	// Wait for all remaining certificates before deploying pods
	for _, cert := range []*certmanagerv1.Certificate{
		s.serverCertificate,
		s.userCertificate,
		s.rustfsCertificate,
		s.otelCollectorCert,
		s.otelServerClientCert,
		s.otelClusterClientCert,
	} {
		err = wait.For(
			conditions.CertificateIsReady(r, cert),
			wait.WithTimeout(5*time.Minute),
			wait.WithInterval(5*time.Second),
		)
		require.NoError(t, err, "certificate %s not ready", cert.Name)
	}

	// Deploy RustFS and OTEL Collector in parallel
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		t.Log("Deploying RustFS infrastructure...")
		require.NoError(t, r.Create(gCtx, s.rustfsService), "failed to create RustFS service")
		require.NoError(t, r.Create(gCtx, s.rustfsDeployment), "failed to create RustFS deployment")

		if err := wait.For(
			conditions.DeploymentIsReady(r, s.rustfsDeployment),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		); err != nil {
			return fmt.Errorf("RustFS deployment not ready: %w", err)
		}

		t.Log("Creating S3 bucket in RustFS...")
		require.NoError(t, r.Create(gCtx, s.rustfsCreateBucketJob), "failed to create bucket creation job")

		return wait.For(
			conditions.JobIsComplete(r, s.rustfsCreateBucketJob),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(10*time.Second),
		)
	})

	g.Go(func() error {
		t.Log("Deploying OTEL Collector...")
		require.NoError(t, r.Create(gCtx, s.otelDeployment), "failed to create OTEL deployment")
		require.NoError(t, r.Create(gCtx, s.otelService), "failed to create OTEL service")

		return wait.For(
			conditions.DeploymentIsReady(r, s.otelDeployment),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		)
	})

	require.NoError(t, g.Wait(), "parallel deployment failed")

	// Deploy Klio Server after RustFS is ready to avoid S3 connection retries
	t.Log("Deploying Klio Server...")
	require.NoError(t, r.Create(ctx, s.klioServer), "failed to create Klio server")
	err = wait.For(
		conditions.KlioServerIsReady(r, s.klioServer),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "Klio server not ready")

	// Deploy CNPG cluster
	require.NoError(t, r.Create(ctx, s.klioPluginConfiguration), "failed to create Klio plugin configuration")
	require.NoError(t, r.Create(ctx, s.cnpgCluster), "failed to create CNPG cluster")

	t.Log("Waiting for CNPG cluster to be ready")
	err = wait.For(
		machineryConditions.ClusterIsReady(r, s.cnpgCluster),
		wait.WithTimeout(4*time.Minute),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "failed to wait for CNPG cluster to be ready")

	t.Logf("Resources created and ready for OTEL metrics test: %s", s.name)

	return ctx
}

func (s *otelMetricsScenario) Teardown(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
) context.Context {
	t.Helper()

	t.Logf("Tearing down resources for OTEL metrics test: %s", s.name)
	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Delete cluster-scoped resources
	_ = r.Delete(ctx, s.otelClusterRoleBinding)
	_ = r.Delete(ctx, s.otelClusterRole)

	// Delete namespace (will cascade delete namespaced resources)
	require.NoError(t, r.Delete(ctx, s.namespace), "failed to delete namespace")

	t.Logf("Resources torn down for OTEL metrics test: %s", s.name)

	return ctx
}

// fetchCollectorMetrics fetches metrics from the OTEL Collector's Prometheus endpoint.
func fetchCollectorMetrics(
	ctx context.Context,
	restConfig *rest.Config,
	namespace, podName string,
) (metrics.PrometheusMetrics, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Use the Kubernetes API pod proxy to fetch metrics from inside the cluster.
	// The port must be embedded in the pod name (podName:port) for the proxy URL
	// to be routed correctly: /api/v1/namespaces/{ns}/pods/{name}:{port}/proxy/...
	req := clientset.CoreV1().RESTClient().Get().
		Resource("pods").
		Name(fmt.Sprintf("%s:%d", podName, otel.CollectorMetricsPort)).
		Namespace(namespace).
		SubResource("proxy").
		Suffix("/metrics")

	result := req.Do(ctx)
	if result.Error() != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", result.Error())
	}

	body, err := result.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics response: %w", err)
	}

	return metrics.ParsePrometheusMetrics(bytes.NewReader(body))
}

// assertOTELMetricsReceived verifies that the OTEL Collector received expected metrics.
func assertOTELMetricsReceived(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	scenario *otelMetricsScenario,
) {
	t.Helper()

	// Get the OTEL Collector pod
	clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create kubernetes clientset")

	podList, err := clientset.CoreV1().Pods(scenario.namespace.Name).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + otel.CollectorName,
	})
	require.NoError(t, err, "failed to list OTEL collector pods")
	require.Len(t, podList.Items, 1, "expected exactly one OTEL collector pod")

	collectorPod := &podList.Items[0]

	t.Log("Fetching metrics from OTEL Collector Prometheus endpoint")

	// Fetch and parse metrics
	promMetrics, err := fetchCollectorMetrics(ctx, cfg.Client().RESTConfig(),
		scenario.namespace.Name, collectorPod.Name)
	require.NoError(t, err, "failed to fetch metrics from OTEL collector")

	t.Logf("Found %d metrics from OTEL Collector", len(promMetrics))

	// Log all klio metrics for debugging
	klioMetrics := promMetrics.FindByNamePrefix("klio_")
	t.Logf("Found %d klio_* metrics", len(klioMetrics))
	for _, m := range klioMetrics {
		t.Logf("  %s = %v", m.Name, m.Value)
	}

	// Verify backup lifecycle metrics exist and have expected values
	t.Log("Verifying backup lifecycle metrics")

	// After a successful backup, these metrics should exist
	assert.True(t, promMetrics.HasMetric("klio_backup_running"),
		"metric klio_backup_running not found")
	assert.True(t, promMetrics.HasMetric("klio_backup_successes_total"),
		"metric klio_backup_successes_total not found")
	assert.True(t, promMetrics.HasMetric("klio_backup_verifications_total"),
		"metric klio_backup_verifications_total not found")
	assert.True(t, promMetrics.HasMetric("klio_backup_latest_start_time_seconds"),
		"metric klio_backup_latest_start_time_seconds not found")
	assert.True(t, promMetrics.HasMetric("klio_backup_latest_completion_time_seconds"),
		"metric klio_backup_latest_completion_time_seconds not found")
	assert.True(t, promMetrics.HasMetric("klio_backup_latest_duration_seconds"),
		"metric klio_backup_latest_duration_seconds not found")

	// Validate metric values
	running, found := promMetrics.GetValue("klio_backup_running")
	if assert.True(t, found, "klio_backup_running not found") {
		assert.InDelta(t, 0, running, 0.001, "backup should not be running after completion")
	}

	successes, found := promMetrics.GetValue("klio_backup_successes_total")
	if assert.True(t, found, "klio_backup_successes_total not found") {
		assert.GreaterOrEqual(t, successes, float64(1), "should have at least 1 successful backup")
	}

	verifications, found := promMetrics.GetValue("klio_backup_verifications_total")
	if assert.True(t, found, "klio_backup_verifications_total not found") {
		assert.GreaterOrEqual(t, verifications, float64(1), "should have at least 1 verification")
	}

	duration, found := promMetrics.GetValue("klio_backup_latest_duration_seconds")
	if assert.True(t, found, "klio_backup_latest_duration_seconds not found") {
		assert.Greater(t, duration, float64(0), "backup duration should be positive")
	}

	startTime, found := promMetrics.GetValue("klio_backup_latest_start_time_seconds")
	if assert.True(t, found, "klio_backup_latest_start_time_seconds not found") {
		assert.Greater(t, startTime, float64(0), "start time should be a valid epoch")
	}

	completionTime, found := promMetrics.GetValue("klio_backup_latest_completion_time_seconds")
	if assert.True(t, found, "klio_backup_latest_completion_time_seconds not found") {
		assert.Greater(t, completionTime, float64(0), "completion time should be a valid epoch")
		assert.GreaterOrEqual(t, completionTime, startTime,
			"completion time should be >= start time")
	}

	// Verify no failures (GetValue returns 0 when the metric is absent).
	failures, _ := promMetrics.GetValue("klio_backup_failures_total")
	assert.InDelta(t, 0, failures, 0.001, "should have no backup failures")

	// Verify NATS queue depth metrics are exported.
	assert.True(t, promMetrics.HasMetric("klio_queue_messages"),
		"metric klio_queue_messages not found")
	assert.True(t, promMetrics.HasMetric("klio_queue_bytes"),
		"metric klio_queue_bytes not found")

	queueMessages, found := promMetrics.GetValue("klio_queue_messages")
	if assert.True(t, found, "klio_queue_messages not found") {
		assert.GreaterOrEqual(t, queueMessages, float64(0),
			"queue message count should be non-negative")
	}

	queueBytes, found := promMetrics.GetValue("klio_queue_bytes")
	if assert.True(t, found, "klio_queue_bytes not found") {
		assert.GreaterOrEqual(t, queueBytes, float64(0),
			"queue byte count should be non-negative")
	}

	// Verify WAL metrics (Tier 1 and Tier 2)
	assertOTELWALMetrics(t, cfg, scenario, promMetrics, collectorPod)

	t.Log("OTEL metrics verification completed successfully")
}

// assertOTELWALMetrics verifies WAL-related metrics from Tier 1 and Tier 2.
func assertOTELWALMetrics(
	t *testing.T,
	cfg *envconf.Config,
	scenario *otelMetricsScenario,
	promMetrics metrics.PrometheusMetrics,
	collectorPod *corev1.Pod,
) {
	t.Helper()

	// Verify consumer WAL written metric (Tier 2 WAL archiving)
	t.Log("Verifying consumer WAL written metric")
	consumerWritten, found := promMetrics.GetValue("klio_consumer_written_total")
	t.Logf("klio_consumer_written_total = %v (found: %v)", consumerWritten, found)
	if assert.True(t, found, "metric klio_consumer_written_total not found") {
		assert.GreaterOrEqual(t, consumerWritten, float64(1),
			"should have at least 1 WAL written by consumer")
	}

	// Verify WAL server latest written time (Tier 1)
	walLatestWritten, found := promMetrics.GetValue("klio_wal_latest_written_time_seconds")
	t.Logf("klio_wal_latest_written_time_seconds = %v (found: %v)", walLatestWritten, found)
	if assert.True(t, found, "metric klio_wal_latest_written_time_seconds not found") {
		assert.Greater(t, walLatestWritten, float64(0),
			"WAL latest written time should be a valid epoch")
	}

	// Verify consumer latest written time (Tier 2). The consumer processes
	// WALs asynchronously so the metric may not be present yet; poll for it.
	t.Log("Waiting for consumer latest written time metric")
	err := wait.For(
		func(ctx context.Context) (bool, error) {
			freshMetrics, fetchErr := fetchCollectorMetrics(ctx, cfg.Client().RESTConfig(),
				scenario.namespace.Name, collectorPod.Name)
			if fetchErr != nil {
				return false, nil //nolint:nilerr // retry on transient errors
			}

			consumerLatestWritten, found := freshMetrics.GetValue("klio_consumer_latest_written_time_seconds")
			if !found {
				return false, nil
			}

			t.Logf("  klio_consumer_latest_written_time_seconds = %v", consumerLatestWritten)
			assert.Greater(t, consumerLatestWritten, float64(0),
				"consumer latest written time should be a valid epoch")

			return true, nil
		},
		wait.WithTimeout(90*time.Second),
		wait.WithInterval(10*time.Second),
	)
	require.NoError(t, err, "metric klio_consumer_latest_written_time_seconds not found within timeout")
}

// assertOTELTracesReceived verifies that the OTEL Collector received expected traces.
// It parses the collector's debug exporter logs to find trace spans.
func assertOTELTracesReceived(
	ctx context.Context,
	t *testing.T,
	cfg *envconf.Config,
	scenario *otelMetricsScenario,
) {
	t.Helper()

	r, err := resources.New(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create resources client")

	// Get the OTEL Collector pod
	clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
	require.NoError(t, err, "failed to create kubernetes clientset")

	podList, err := clientset.CoreV1().Pods(scenario.namespace.Name).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + otel.CollectorName,
	})
	require.NoError(t, err, "failed to list OTEL collector pods")
	require.Len(t, podList.Items, 1, "expected exactly one OTEL collector pod")

	collectorPod := &podList.Items[0]

	t.Log("Fetching traces from OTEL Collector debug logs")

	// Get collector logs (traces are exported via debug exporter)
	logs, err := pods.GetLogs(ctx, r, collectorPod, "collector")
	require.NoError(t, err, "failed to get OTEL collector logs")

	// Verify backup lifecycle spans are present
	t.Log("Verifying backup lifecycle spans")

	expectedSpans := []string{
		"backup",
		"backup_run",
		"backup_verify",
	}

	for _, span := range expectedSpans {
		assert.Contains(t, logs, span,
			"expected span %q not found in OTEL collector logs", span)
	}

	// Verify resource attributes are present
	t.Log("Verifying resource attributes in traces")

	expectedAttributes := []string{
		"service.name",
		"telemetry.sdk.language",
		"telemetry.sdk.name",
	}

	for _, attr := range expectedAttributes {
		assert.Contains(t, logs, attr,
			"expected resource attribute %q not found in OTEL collector logs", attr)
	}

	// Verify trace context propagation.
	// The OTEL collector debug exporter formats span fields as "Trace ID" and "Parent ID".
	assert.Contains(t, logs, "Trace ID",
		"expected TraceID in trace output")
	assert.Contains(t, logs, "Parent ID",
		"expected SpanID in trace output")

	t.Log("OTEL traces verification completed successfully")
}

// OTELMetricsAndTraces returns a BackupFeature that tests OTEL metrics and traces collection.
func OTELMetricsAndTraces(namespace string) *machineryFeatures.BackupFeature {
	scenario := newOTELMetricsScenario(namespace)

	return machineryFeatures.NewBackupFeature(machineryFeatures.BackupFeatureConfig{
		Name:     "OTELMetricsAndTraces",
		Setup:    scenario.Setup,
		Teardown: scenario.Teardown,
		Backup:   scenario.backup,
		PostBackupAssert: func(ctx context.Context, t *testing.T, cfg *envconf.Config, _ *cnpgv1.Backup) {
			t.Helper()
			// Wait a bit for telemetry to be exported
			time.Sleep(10 * time.Second)
			assertOTELMetricsReceived(ctx, t, cfg, scenario)
			assertOTELTracesReceived(ctx, t, cfg, scenario)
		},
		BackupTimeout: 2 * time.Minute,
	})
}

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	"github.com/cloudnative-pg/klio/operator/test/utils/metrics"
)

const (
	operatorNamespace      = "cnpg-system"
	operatorDeploymentName = "klio-controller-manager"
	operatorOTELCollector  = "operator-otel-collector"
	// collectorImage must match the image used in otel.go.
	//nolint:godot,lll
	// renovate image: datasource=docker depName=otel/opentelemetry-collector-contrib versioning=docker
	collectorImage       = "otel/opentelemetry-collector-contrib:0.153.0@sha256:93aad750175cbf1a973ae1c5886c3371f4d800f61be25cdd26870b8441ffe9fa"
	collectorHTTPPort    = 4318
	collectorMetricsPort = 9464
)

// operatorOTELFeature implements machineryFeatures.Feature.
type operatorOTELFeature struct {
	namespace string
	// savedEnv preserves the original env vars so teardown can restore them.
	savedEnv []corev1.EnvVar
}

func (f *operatorOTELFeature) Name() string { return "OperatorOTELMetrics" }

func (f *operatorOTELFeature) Setup() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()

		clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
		require.NoError(t, err)

		// Create the test namespace.
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: f.namespace}}
		_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		require.NoError(t, err, "failed to create test namespace")

		// Deploy a minimal insecure OTel collector.
		t.Log("Deploying insecure OTel collector for operator metrics")
		require.NoError(t, createOperatorOTELCollector(ctx, clientset, f.namespace))

		// Wait for the collector to be ready.
		require.NoError(t, wait.For(
			func(ctx context.Context) (bool, error) {
				dep, err := clientset.AppsV1().Deployments(f.namespace).Get(ctx, operatorOTELCollector, metav1.GetOptions{})
				if err != nil {
					return false, nil //nolint:nilerr // retry on transient errors
				}

				return dep.Status.ReadyReplicas > 0, nil
			},
			wait.WithContext(ctx),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		), "OTel collector not ready")

		// Patch the operator deployment with OTEL_* env vars.
		t.Log("Patching operator deployment with OTEL env vars")
		dep, err := clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorDeploymentName, metav1.GetOptions{})
		require.NoError(t, err, "failed to get operator deployment")

		// Save the original env for teardown.
		f.savedEnv = make([]corev1.EnvVar, len(dep.Spec.Template.Spec.Containers[0].Env))
		copy(f.savedEnv, dep.Spec.Template.Spec.Containers[0].Env)

		collectorEndpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
			operatorOTELCollector, f.namespace, collectorHTTPPort)

		otelEnv := []corev1.EnvVar{
			{Name: "OTEL_SERVICE_NAME", Value: "klio-operator"},
			{Name: "OTEL_METRICS_EXPORTER", Value: "otlp"},
			{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf"},
			{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: collectorEndpoint},
			{Name: "OTEL_METRIC_EXPORT_INTERVAL", Value: "5000"},
		}
		dep.Spec.Template.Spec.Containers[0].Env = append(dep.Spec.Template.Spec.Containers[0].Env, otelEnv...)

		_, err = clientset.AppsV1().Deployments(operatorNamespace).Update(ctx, dep, metav1.UpdateOptions{})
		require.NoError(t, err, "failed to patch operator deployment")

		// Wait for the rollout to complete.
		t.Log("Waiting for operator rollout")
		require.NoError(t, wait.For(
			func(ctx context.Context) (bool, error) {
				dep, err := clientset.AppsV1().Deployments(operatorNamespace).Get(
					ctx, operatorDeploymentName, metav1.GetOptions{})
				if err != nil {
					return false, nil //nolint:nilerr // retry on transient errors
				}

				return dep.Status.UpdatedReplicas == *dep.Spec.Replicas &&
					dep.Status.ReadyReplicas == *dep.Spec.Replicas &&
					dep.Status.ObservedGeneration >= dep.Generation, nil
			},
			wait.WithContext(ctx),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		), "operator rollout not complete")

		return ctx
	}
}

func (f *operatorOTELFeature) Run() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()

		clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
		require.NoError(t, err)

		// Allow time for metrics to be exported.
		t.Log("Waiting for metrics to propagate")
		time.Sleep(15 * time.Second)

		// Fetch the collector pod.
		podList, err := clientset.CoreV1().Pods(f.namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + operatorOTELCollector,
		})
		require.NoError(t, err)
		require.Len(t, podList.Items, 1, "expected one collector pod")

		collectorPod := podList.Items[0].Name

		// Fetch metrics from the collector's Prometheus endpoint.
		t.Log("Fetching metrics from operator OTel collector")
		req := clientset.CoreV1().RESTClient().Get().
			Resource("pods").
			Name(fmt.Sprintf("%s:%d", collectorPod, collectorMetricsPort)).
			Namespace(f.namespace).
			SubResource("proxy").
			Suffix("/metrics")

		body, err := req.Do(ctx).Raw()
		require.NoError(t, err, "failed to fetch collector metrics")

		promMetrics, err := metrics.ParsePrometheusMetrics(bytes.NewReader(body))
		require.NoError(t, err, "failed to parse collector metrics")

		t.Logf("Found %d total metrics from operator OTel collector", len(promMetrics))

		// Log all metric names for debugging.
		t.Logf("All metric names: %v", promMetrics.Names())

		// Assert controller-runtime metrics are bridged through OTel.
		// The exact set depends on what the operator has done since restart;
		// webhook panic counters are always registered.
		crMetrics := promMetrics.FindByNamePrefix("controller_runtime_")
		t.Logf("Found %d controller_runtime_* metrics", len(crMetrics))
		for _, m := range crMetrics {
			t.Logf("  %s = %v", m.Name, m.Value)
		}
		assert.NotEmpty(t, crMetrics,
			"expected at least one controller_runtime_* metric from the Prometheus bridge")

		// Verify service.name resource attribute propagation on any
		// controller-runtime metric that was collected.
		if len(crMetrics) > 0 {
			serviceNameLabels := map[string]string{"service_name": "klio-operator"}
			_, found := promMetrics.GetValueWithLabels(crMetrics[0].Name, serviceNameLabels)
			assert.True(t, found,
				"%s should carry service_name=klio-operator label", crMetrics[0].Name)
		}

		t.Log("Operator OTel metrics verification completed")

		return ctx
	}
}

func (f *operatorOTELFeature) Teardown() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()

		clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
		require.NoError(t, err)

		// Restore the operator deployment's original env vars.
		t.Log("Restoring operator deployment")
		dep, err := clientset.AppsV1().Deployments(operatorNamespace).Get(ctx, operatorDeploymentName, metav1.GetOptions{})
		if err == nil {
			dep.Spec.Template.Spec.Containers[0].Env = f.savedEnv
			_, err = clientset.AppsV1().Deployments(operatorNamespace).Update(ctx, dep, metav1.UpdateOptions{})
			if err != nil {
				t.Logf("failed to restore operator deployment: %v", err)
			}
		}

		// Wait for the rollout after restore.
		_ = wait.For(
			func(ctx context.Context) (bool, error) {
				dep, err := clientset.AppsV1().Deployments(operatorNamespace).Get(
					ctx, operatorDeploymentName, metav1.GetOptions{})
				if err != nil {
					return false, nil //nolint:nilerr // retry on transient errors
				}

				return dep.Status.UpdatedReplicas == *dep.Spec.Replicas &&
					dep.Status.ReadyReplicas == *dep.Spec.Replicas &&
					dep.Status.ObservedGeneration >= dep.Generation, nil
			},
			wait.WithContext(ctx),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		)

		// Clean up the test namespace.
		_ = clientset.CoreV1().Namespaces().Delete(ctx, f.namespace, metav1.DeleteOptions{})

		return ctx
	}
}

// OperatorOTELMetrics returns a Feature that verifies the operator exports
// controller-runtime metrics through the OpenTelemetry bridge.
func OperatorOTELMetrics(namespace string) *operatorOTELFeature {
	return &operatorOTELFeature{namespace: namespace}
}

// createOperatorOTELCollector deploys a minimal insecure OTel collector for
// receiving metrics from the operator over HTTP/protobuf.
func createOperatorOTELCollector(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	labels := map[string]string{"app": operatorOTELCollector}

	config := `receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
processors:
  batch:
    send_batch_size: 100
    timeout: 1s
exporters:
  prometheus:
    endpoint: "0.0.0.0:9464"
    send_timestamps: true
    resource_to_telemetry_conversion:
      enabled: true
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [prometheus]
`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorOTELCollector + "-config",
			Namespace: namespace,
		},
		Data: map[string]string{"config.yaml": config},
	}
	if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create collector configmap: %w", err)
	}

	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorOTELCollector,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "collector",
						Image: collectorImage,
						Args:  []string{"--config=/etc/otel/config.yaml"},
						Ports: []corev1.ContainerPort{
							{Name: "otlp-http", ContainerPort: collectorHTTPPort, Protocol: corev1.ProtocolTCP},
							{Name: "metrics", ContainerPort: collectorMetricsPort, Protocol: corev1.ProtocolTCP},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "config",
							MountPath: "/etc/otel",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: operatorOTELCollector + "-config",
								},
							},
						},
					}},
				},
			},
		},
	}
	if _, err := clientset.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create collector deployment: %w", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorOTELCollector,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "otlp-http", Port: collectorHTTPPort, TargetPort: intstr.FromInt32(collectorHTTPPort)},
				{Name: "metrics", Port: collectorMetricsPort, TargetPort: intstr.FromInt32(collectorMetricsPort)},
			},
		},
	}
	if _, err := clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create collector service: %w", err)
	}

	return nil
}

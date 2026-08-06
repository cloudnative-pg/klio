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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/types"

	"github.com/cloudnative-pg/klio/operator/test/klio/testconfig"
	machineryConditions "github.com/cloudnative-pg/klio/operator/test/machinery/pkg/conditions"
	"github.com/cloudnative-pg/klio/operator/test/machinery/pkg/namespaces"
	"github.com/cloudnative-pg/klio/operator/test/utils/metrics"
)

const (
	operatorOTELCollector = "operator-otel-collector"
	// collectorImage must match the image used in otel.go.
	//nolint:godot,lll
	// renovate image: datasource=docker depName=otel/opentelemetry-collector-contrib versioning=docker
	collectorImage       = "otel/opentelemetry-collector-contrib:0.158.0@sha256:c5918f78992ee73b0d6f0e599423ac5ec52dd5d9726733114d6eca53d5a32ed5"
	collectorHTTPPort    = 4318
	collectorMetricsPort = 9464
)

// operatorOTELFeature implements machineryFeatures.Feature.
type operatorOTELFeature struct {
	namespace string
	// operatorNamespace is where the Klio operator runs (cnpg-system on Kind,
	// openshift-operators on OpenShift).
	operatorNamespace string
	// operatorAppLabel is the label selector identifying the operator Deployment
	// (app.kubernetes.io/name=klio on Kind, =klio-operator on OpenShift).
	operatorAppLabel string
	// operatorSubscription, when non-empty, is the name of the OLM Subscription
	// (in operatorNamespace) managing the operator. On OpenShift the operator is
	// OLM-managed, so the OTEL env must be set on the Subscription's
	// spec.config.env (which OLM propagates to the Deployment); patching the
	// Deployment directly is reverted by OLM. Empty means a Helm/Kind install
	// whose Deployment is patched directly.
	operatorSubscription string
	// savedEnv preserves the original Deployment env so teardown can restore it
	// (Helm/Kind path).
	savedEnv []corev1.EnvVar
	// savedSubEnv preserves the original Subscription spec.config.env so teardown
	// can restore it (OLM path). nil when the field was absent.
	savedSubEnv []any
}

// subscriptionGVR is the OLM Subscription resource.
//
//nolint:gochecknoglobals
var subscriptionGVR = schema.GroupVersionResource{
	Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions",
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

		collectorEndpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
			operatorOTELCollector, f.namespace, collectorHTTPPort)

		otelEnv := []corev1.EnvVar{
			{Name: "OTEL_SERVICE_NAME", Value: "klio-operator"},
			{Name: "OTEL_METRICS_EXPORTER", Value: "otlp"},
			{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf"},
			{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: collectorEndpoint},
			{Name: "OTEL_METRIC_EXPORT_INTERVAL", Value: "5000"},
		}

		if f.operatorSubscription != "" {
			// OLM-managed operator (OpenShift): set the env on the Subscription's
			// spec.config.env so OLM propagates it to the Deployment. Patching the
			// Deployment directly is reverted by OLM within seconds.
			t.Log("Injecting OTEL env via the OLM Subscription")
			dyn, err := dynamic.NewForConfig(cfg.Client().RESTConfig())
			require.NoError(t, err)

			f.savedSubEnv, err = setSubscriptionEnv(
				ctx, dyn, f.operatorNamespace, f.operatorSubscription, otelEnv)
			require.NoError(t, err, "failed to set OTEL env on the operator Subscription")

			// OLM updates the Deployment asynchronously; wait until it carries
			// the env before waiting on the rollout.
			t.Log("Waiting for OLM to propagate the env to the operator deployment")
			require.NoError(t, waitForDeploymentEnv(
				ctx, clientset, f.operatorNamespace, f.operatorAppLabel, "OTEL_SERVICE_NAME"),
				"OLM did not propagate the OTEL env to the operator deployment")
		} else {
			// Helm/Kind: patch the Deployment directly and save the original env.
			t.Log("Patching operator deployment with OTEL env vars")
			dep, err := operatorDeployment(ctx, clientset, f.operatorNamespace, f.operatorAppLabel)
			require.NoError(t, err, "failed to find operator deployment")

			f.savedEnv = make([]corev1.EnvVar, len(dep.Spec.Template.Spec.Containers[0].Env))
			copy(f.savedEnv, dep.Spec.Template.Spec.Containers[0].Env)

			dep.Spec.Template.Spec.Containers[0].Env = append(
				dep.Spec.Template.Spec.Containers[0].Env, otelEnv...)
			_, err = clientset.AppsV1().Deployments(f.operatorNamespace).Update(ctx, dep, metav1.UpdateOptions{})
			require.NoError(t, err, "failed to patch operator deployment")
		}

		// Wait for the rollout to complete.
		t.Log("Waiting for operator rollout")
		dep, err := operatorDeployment(ctx, clientset, f.operatorNamespace, f.operatorAppLabel)
		require.NoError(t, err, "failed to find operator deployment")
		require.NoError(t, wait.For(
			machineryConditions.DeploymentRolloutComplete(cfg.Client().Resources(), dep),
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

		// Poll the collector until the operator's controller-runtime metrics
		// reach it through the Prometheus->OTLP bridge, instead of assuming a
		// fixed delay: the export interval, OTLP delivery, batch flush, and
		// Prometheus scrape each add latency that varies with cluster load.
		t.Log("Waiting for operator controller-runtime metrics to propagate")
		var promMetrics, crMetrics metrics.PrometheusMetrics
		require.NoError(t, wait.For(
			func(ctx context.Context) (bool, error) {
				pods, err := clientset.CoreV1().Pods(f.namespace).List(ctx, metav1.ListOptions{
					LabelSelector: "app=" + operatorOTELCollector,
				})
				if err != nil || len(pods.Items) != 1 {
					return false, nil //nolint:nilerr // retry until the collector pod is up
				}

				body, err := clientset.CoreV1().RESTClient().Get().
					Resource("pods").
					Name(fmt.Sprintf("%s:%d", pods.Items[0].Name, collectorMetricsPort)).
					Namespace(f.namespace).
					SubResource("proxy").
					Suffix("/metrics").
					Do(ctx).Raw()
				if err != nil {
					return false, nil //nolint:nilerr // retry on transient proxy errors
				}

				parsed, err := metrics.ParsePrometheusMetrics(bytes.NewReader(body))
				if err != nil {
					return false, nil //nolint:nilerr // retry on a partial scrape
				}

				promMetrics = parsed
				// The exact set depends on what the operator has done since
				// restart; we only need the bridge to have delivered at least one.
				crMetrics = parsed.FindByNamePrefix("controller_runtime_")

				return len(crMetrics) > 0, nil
			},
			wait.WithContext(ctx),
			wait.WithTimeout(2*time.Minute),
			wait.WithInterval(5*time.Second),
		), "no controller_runtime_* metrics reached the OTel collector")

		t.Logf("Found %d total metrics from operator OTel collector", len(promMetrics))
		t.Logf("All metric names: %v", promMetrics.Names())
		t.Logf("Found %d controller_runtime_* metrics", len(crMetrics))
		for _, m := range crMetrics {
			t.Logf("  %s = %v", m.Name, m.Value)
		}

		// Verify service.name resource attribute propagation on a collected
		// metric (wait.For above guarantees crMetrics is non-empty here).
		serviceNameLabels := map[string]string{"service_name": "klio-operator"}
		_, found := promMetrics.GetValueWithLabels(crMetrics[0].Name, serviceNameLabels)
		assert.True(t, found,
			"%s should carry service_name=klio-operator label", crMetrics[0].Name)

		t.Log("Operator OTel metrics verification completed")

		return ctx
	}
}

func (f *operatorOTELFeature) Teardown() types.StepFunc {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		t.Helper()

		clientset, err := kubernetes.NewForConfig(cfg.Client().RESTConfig())
		require.NoError(t, err)

		// Restore the operator's original env. On OLM we reset the Subscription
		// (which OLM propagates back to the Deployment); otherwise we restore the
		// Deployment directly.
		t.Log("Restoring operator environment")
		if f.operatorSubscription != "" {
			dyn, err := dynamic.NewForConfig(cfg.Client().RESTConfig())
			require.NoError(t, err)
			require.NoError(t, restoreSubscriptionEnv(
				ctx, dyn, f.operatorNamespace, f.operatorSubscription, f.savedSubEnv),
				"failed to restore the operator Subscription env")
		} else {
			dep, err := operatorDeployment(ctx, clientset, f.operatorNamespace, f.operatorAppLabel)
			require.NoError(t, err, "failed to fetch operator deployment")

			dep.Spec.Template.Spec.Containers[0].Env = f.savedEnv
			_, err = clientset.AppsV1().Deployments(f.operatorNamespace).Update(
				ctx, dep, metav1.UpdateOptions{})
			require.NoError(t, err, "failed to restore operator deployment")
		}

		// Wait for the rollout after restore.
		if dep, err := operatorDeployment(ctx, clientset, f.operatorNamespace, f.operatorAppLabel); err == nil {
			_ = wait.For(
				machineryConditions.DeploymentRolloutComplete(cfg.Client().Resources(), dep),
				wait.WithContext(ctx),
				wait.WithTimeout(2*time.Minute),
				wait.WithInterval(5*time.Second),
			)
		}

		// Clean up the test namespace.
		namespaces.DumpNamespaceOnFailure(
			ctx, t, cfg.Client().Resources(), testCfg.LogDir, f.namespace, testconfig.DumpedKinds())
		_ = clientset.CoreV1().Namespaces().Delete(ctx, f.namespace, metav1.DeleteOptions{})

		return ctx
	}
}

// OperatorOTELMetrics returns a Feature that verifies the operator exports
// controller-runtime metrics through the OpenTelemetry bridge. operatorNamespace
// is where the Klio operator runs and operatorAppLabel selects its Deployment.
// operatorSubscription, when non-empty, names the OLM Subscription managing the
// operator (OpenShift): the OTEL env is then set on the Subscription rather than
// the Deployment, since OLM reverts direct Deployment patches.
func OperatorOTELMetrics(
	namespace, operatorNamespace, operatorAppLabel, operatorSubscription string,
) *operatorOTELFeature {
	return &operatorOTELFeature{
		namespace:            namespace,
		operatorNamespace:    operatorNamespace,
		operatorAppLabel:     operatorAppLabel,
		operatorSubscription: operatorSubscription,
	}
}

// setSubscriptionEnv merges otelEnv into the OLM Subscription's
// spec.config.env, dropping any pre-existing OTEL_* entries and preserving the
// rest (e.g. SIDECAR_IMAGE). It returns the original env slice so teardown can
// restore it; the result is nil when spec.config.env was absent.
func setSubscriptionEnv(
	ctx context.Context, dyn dynamic.Interface, namespace, name string, otelEnv []corev1.EnvVar,
) ([]any, error) {
	sub, err := dyn.Resource(subscriptionGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting subscription %s/%s: %w", namespace, name, err)
	}

	original, _, err := unstructured.NestedSlice(sub.Object, "spec", "config", "env")
	if err != nil {
		return nil, fmt.Errorf("reading spec.config.env: %w", err)
	}

	merged := make([]any, 0, len(original)+len(otelEnv))
	for _, e := range original {
		if m, ok := e.(map[string]any); ok {
			if n, _ := m["name"].(string); strings.HasPrefix(n, "OTEL_") {
				continue
			}
		}
		merged = append(merged, e)
	}
	for _, e := range otelEnv {
		merged = append(merged, map[string]any{"name": e.Name, "value": e.Value})
	}

	if err := unstructured.SetNestedSlice(sub.Object, merged, "spec", "config", "env"); err != nil {
		return nil, fmt.Errorf("setting spec.config.env: %w", err)
	}
	if _, err := dyn.Resource(subscriptionGVR).Namespace(namespace).Update(
		ctx, sub, metav1.UpdateOptions{}); err != nil {
		return nil, fmt.Errorf("updating subscription: %w", err)
	}

	return original, nil
}

// restoreSubscriptionEnv resets the Subscription's spec.config.env to saved
// (nil removes the field).
func restoreSubscriptionEnv(
	ctx context.Context, dyn dynamic.Interface, namespace, name string, saved []any,
) error {
	sub, err := dyn.Resource(subscriptionGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting subscription %s/%s: %w", namespace, name, err)
	}
	if saved == nil {
		unstructured.RemoveNestedField(sub.Object, "spec", "config", "env")
	} else if err := unstructured.SetNestedSlice(sub.Object, saved, "spec", "config", "env"); err != nil {
		return fmt.Errorf("setting spec.config.env: %w", err)
	}
	if _, err := dyn.Resource(subscriptionGVR).Namespace(namespace).Update(
		ctx, sub, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating subscription: %w", err)
	}

	return nil
}

// waitForDeploymentEnv polls until the operator Deployment's first container
// carries an env var named envName, confirming OLM propagated the Subscription
// change to the Deployment.
func waitForDeploymentEnv(
	ctx context.Context, clientset kubernetes.Interface, namespace, labelSelector, envName string,
) error {
	return wait.For(
		func(ctx context.Context) (bool, error) {
			dep, err := operatorDeployment(ctx, clientset, namespace, labelSelector)
			if err != nil {
				return false, nil //nolint:nilerr // retry until OLM settles
			}
			for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
				if e.Name == envName {
					return true, nil
				}
			}

			return false, nil
		},
		wait.WithContext(ctx),
		wait.WithTimeout(2*time.Minute),
		wait.WithInterval(5*time.Second),
	)
}

// operatorDeployment finds the Klio operator Deployment in namespace by label.
// Its name differs across install methods (Helm on Kind, OLM on OpenShift), so
// we locate it by label rather than a fixed name.
func operatorDeployment(
	ctx context.Context, clientset kubernetes.Interface, namespace, labelSelector string,
) (*appsv1.Deployment, error) {
	list, err := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("listing operator deployment in %s: %w", namespace, err)
	}
	if len(list.Items) != 1 {
		return nil, fmt.Errorf("expected exactly one deployment matching %q in %s, found %d",
			labelSelector, namespace, len(list.Items))
	}

	return &list.Items[0], nil
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

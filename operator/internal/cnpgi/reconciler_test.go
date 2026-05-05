package cnpgi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/reconciler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	kliov1alpha1 "github.com/cloudnative-pg/klio/operator/api/v1alpha1"
	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

func newReconcilerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, kliov1alpha1.AddToScheme(scheme))
	require.NoError(t, cnpgv1.AddToScheme(scheme))

	return scheme
}

func buildReconcilerRequest(t *testing.T, cluster *cnpgv1.Cluster) *reconciler.ReconcilerHooksRequest {
	t.Helper()
	clusterJSON, err := json.Marshal(cluster)
	require.NoError(t, err)

	return &reconciler.ReconcilerHooksRequest{
		ClusterDefinition: clusterJSON,
	}
}

func TestReconcilerPre(t *testing.T) {
	scheme := newReconcilerTestScheme(t)

	t.Run("returns BEHAVIOR_REQUEUE when PluginConfiguration does not exist", func(t *testing.T) {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "postgresql.cnpg.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: cnpgv1.ClusterSpec{
				Plugins: []cnpgv1.PluginConfiguration{
					{
						Name:    klioconfig.PluginName,
						Enabled: new(true),
						Parameters: map[string]string{
							klioconfig.PluginConfigurationRefParam: "missing-plugin-config",
						},
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		impl := ReconcilerImplementation{Client: fakeClient}
		req := buildReconcilerRequest(t, cluster)

		resp, err := impl.Pre(context.Background(), req)

		require.NoError(t, err, "requeue must not return an error")
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_REQUEUE, resp.GetBehavior())
		assert.Equal(t, defaultRequeueAfterSeconds, resp.GetRequeueAfter())
	})

	t.Run("returns BEHAVIOR_CONTINUE when PluginConfiguration exists", func(t *testing.T) {
		pluginConfig := &kliov1alpha1.PluginConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-plugin-config",
				Namespace: "test-ns",
			},
			Spec: kliov1alpha1.PluginConfigurationSpec{
				ClusterName:   "test-cluster",
				ServerAddress: "klio.example",
			},
		}
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "postgresql.cnpg.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: cnpgv1.ClusterSpec{
				Plugins: []cnpgv1.PluginConfiguration{
					{
						Name:    klioconfig.PluginName,
						Enabled: new(true),
						Parameters: map[string]string{
							klioconfig.PluginConfigurationRefParam: "existing-plugin-config",
						},
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(pluginConfig).
			Build()

		impl := ReconcilerImplementation{Client: fakeClient}
		req := buildReconcilerRequest(t, cluster)

		resp, err := impl.Pre(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE, resp.GetBehavior())
	})

	t.Run("returns BEHAVIOR_CONTINUE for cluster without Klio plugin", func(t *testing.T) {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "postgresql.cnpg.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-plugin-cluster",
				Namespace: "test-ns",
			},
			Spec: cnpgv1.ClusterSpec{},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		impl := ReconcilerImplementation{Client: fakeClient}
		req := buildReconcilerRequest(t, cluster)

		resp, err := impl.Pre(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE, resp.GetBehavior())
	})

	t.Run("returns BEHAVIOR_CONTINUE when cluster definition is nil", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		impl := ReconcilerImplementation{Client: fakeClient}
		req := &reconciler.ReconcilerHooksRequest{
			ClusterDefinition: nil,
		}

		resp, err := impl.Pre(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE, resp.GetBehavior())
	})

	t.Run("returns BEHAVIOR_REQUEUE when external cluster PluginConfiguration is missing", func(t *testing.T) {
		archivePC := &kliov1alpha1.PluginConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "archive-plugin-config",
				Namespace: "test-ns",
			},
			Spec: kliov1alpha1.PluginConfigurationSpec{
				ClusterName:   "test-cluster",
				ServerAddress: "klio.example",
			},
		}
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "postgresql.cnpg.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: cnpgv1.ClusterSpec{
				Plugins: []cnpgv1.PluginConfiguration{
					{
						Name:    klioconfig.PluginName,
						Enabled: new(true),
						Parameters: map[string]string{
							klioconfig.PluginConfigurationRefParam: "archive-plugin-config",
						},
					},
				},
				ExternalClusters: []cnpgv1.ExternalCluster{
					{
						Name: "source-cluster",
						PluginConfiguration: &cnpgv1.PluginConfiguration{
							Name:    klioconfig.PluginName,
							Enabled: new(true),
							Parameters: map[string]string{
								klioconfig.PluginConfigurationRefParam: "missing-source-config",
							},
						},
					},
				},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(archivePC).
			Build()

		impl := ReconcilerImplementation{Client: fakeClient}
		req := buildReconcilerRequest(t, cluster)

		resp, err := impl.Pre(context.Background(), req)

		require.NoError(t, err, "requeue must not return an error")
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_REQUEUE, resp.GetBehavior())
		assert.Equal(t, defaultRequeueAfterSeconds, resp.GetRequeueAfter())
	})

	t.Run("propagates non-NotFound errors", func(t *testing.T) {
		cluster := &cnpgv1.Cluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "postgresql.cnpg.io/v1",
				Kind:       "Cluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-ns",
			},
			Spec: cnpgv1.ClusterSpec{
				Plugins: []cnpgv1.PluginConfiguration{
					{
						Name:    klioconfig.PluginName,
						Enabled: new(true),
						Parameters: map[string]string{
							klioconfig.PluginConfigurationRefParam: "some-config",
						},
					},
				},
			},
		}
		injectedErr := errors.New("simulated API server unavailable")
		fakeClient := interceptor.NewClient(
			fake.NewClientBuilder().WithScheme(scheme).Build(),
			interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch,
					key client.ObjectKey, obj client.Object,
					opts ...client.GetOption,
				) error {
					if _, ok := obj.(*kliov1alpha1.PluginConfiguration); ok {
						return injectedErr
					}

					return c.Get(ctx, key, obj, opts...)
				},
			},
		)

		impl := ReconcilerImplementation{Client: fakeClient}
		req := buildReconcilerRequest(t, cluster)

		resp, err := impl.Pre(context.Background(), req)

		require.Error(t, err, "non-NotFound errors must propagate")
		assert.Nil(t, resp)
		assert.ErrorContains(t, err, "simulated API server unavailable")
	})
}

func TestReconcilerPost(t *testing.T) {
	t.Run("returns BEHAVIOR_CONTINUE", func(t *testing.T) {
		impl := ReconcilerImplementation{}
		resp, err := impl.Post(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE, resp.GetBehavior())
	})
}

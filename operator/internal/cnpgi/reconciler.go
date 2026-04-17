package cnpgi

import (
	"context"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/decoder"
	"github.com/cloudnative-pg/cnpg-i/pkg/reconciler"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/klio/operator/internal/klioconfig"
)

// defaultRequeueAfterSeconds is the default number of seconds to wait before
// retrying when a PluginConfiguration is not found. When this value is returned
// with BEHAVIOR_REQUEUE, CNPG's cluster controller will stop the current
// reconciliation loop and schedule a new one after this duration.
const defaultRequeueAfterSeconds int64 = 5

// ReconcilerImplementation implements the CNPG-I reconciler hooks for Klio.
// The Pre hook gates reconciliation when a PluginConfiguration is missing,
// returning BEHAVIOR_REQUEUE so the cluster waits gracefully instead of
// entering an unrecoverable error state.
type ReconcilerImplementation struct {
	reconciler.UnimplementedReconcilerHooksServer

	Client client.Client
}

// GetCapabilities implements the Reconciler interface.
func (r ReconcilerImplementation) GetCapabilities(
	_ context.Context,
	_ *reconciler.ReconcilerHooksCapabilitiesRequest,
) (*reconciler.ReconcilerHooksCapabilitiesResult, error) {
	return &reconciler.ReconcilerHooksCapabilitiesResult{
		ReconcilerCapabilities: []*reconciler.ReconcilerHooksCapability{
			{
				Kind: reconciler.ReconcilerHooksCapability_KIND_CLUSTER,
			},
			{
				Kind: reconciler.ReconcilerHooksCapability_KIND_BACKUP,
			},
		},
	}, nil
}

// Pre is called before each reconciliation loop. It checks that all required
// PluginConfiguration resources exist. When one is missing, it returns
// BEHAVIOR_REQUEUE so CNPG retries without entering an error state.
func (r ReconcilerImplementation) Pre(
	ctx context.Context,
	request *reconciler.ReconcilerHooksRequest,
) (*reconciler.ReconcilerHooksResult, error) {
	contextLogger := log.FromContext(ctx).WithName("klio-pre-reconcile")

	// Only handle cluster reconciliation
	if request.GetClusterDefinition() == nil {
		return &reconciler.ReconcilerHooksResult{
			Behavior: reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE,
		}, nil
	}

	cluster := &cnpgv1.Cluster{}
	if err := decoder.DecodeObjectLenient(request.GetClusterDefinition(), cluster); err != nil {
		contextLogger.Error(err, "Failed to decode cluster definition")

		return nil, err
	}

	// Check if the cluster has Klio plugins configured
	_, err := klioconfig.ResolveClusterPlugins(ctx, r.Client, cluster)
	if err != nil {
		if klioconfig.IsPluginConfigurationNotFound(err) {
			contextLogger.Info("PluginConfiguration not found, requesting requeue",
				"cluster", cluster.Name,
				"error", err.Error())

			return &reconciler.ReconcilerHooksResult{
				Behavior:     reconciler.ReconcilerHooksResult_BEHAVIOR_REQUEUE,
				RequeueAfter: defaultRequeueAfterSeconds,
			}, nil
		}
		contextLogger.Error(err, "Failed to resolve cluster plugins")

		return nil, err
	}

	return &reconciler.ReconcilerHooksResult{
		Behavior: reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE,
	}, nil
}

// Post is called after the reconciliation loop.
func (r ReconcilerImplementation) Post(
	_ context.Context,
	_ *reconciler.ReconcilerHooksRequest,
) (*reconciler.ReconcilerHooksResult, error) {
	return &reconciler.ReconcilerHooksResult{
		Behavior: reconciler.ReconcilerHooksResult_BEHAVIOR_CONTINUE,
	}, nil
}

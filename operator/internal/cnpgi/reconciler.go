package cnpgi

import (
	"context"

	"github.com/cloudnative-pg/cnpg-i/pkg/reconciler"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcilerImplementation implements the capabilities needed for the CNPGI reconciliation .
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

// Pre look interface definition.
func (r ReconcilerImplementation) Pre(
	_ context.Context,
	_ *reconciler.ReconcilerHooksRequest,
) (*reconciler.ReconcilerHooksResult, error) {
	// TODO
	return nil, nil
}

// Post look interface definition.
func (r ReconcilerImplementation) Post(
	_ context.Context,
	_ *reconciler.ReconcilerHooksRequest,
) (*reconciler.ReconcilerHooksResult, error) {
	// TODO
	return nil, nil
}

package cnpgi

import (
	"context"

	"github.com/cloudnative-pg/cnpg-i/pkg/identity"
)

// identityImplementation implements IdentityServer.
type identityImplementation struct {
	identity.UnimplementedIdentityServer

	capabilities []*identity.PluginCapability
}

// GetPluginMetadata implements IdentityServer.
func (i identityImplementation) GetPluginMetadata(
	_ context.Context,
	_ *identity.GetPluginMetadataRequest,
) (*identity.GetPluginMetadataResponse, error) {
	return &data, nil
}

// GetPluginCapabilities implements IdentityServer.
func (i identityImplementation) GetPluginCapabilities(
	_ context.Context,
	_ *identity.GetPluginCapabilitiesRequest,
) (*identity.GetPluginCapabilitiesResponse, error) {
	return &identity.GetPluginCapabilitiesResponse{Capabilities: i.capabilities}, nil
}

// Probe implements IdentityServer.
func (i identityImplementation) Probe(
	_ context.Context,
	_ *identity.ProbeRequest,
) (*identity.ProbeResponse, error) {
	return &identity.ProbeResponse{
		Ready: true,
	}, nil
}

package identity

import (
	"context"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
)

// IdentityProvider abstracts OTC authentication and service client creation.
type IdentityProvider interface {
	// GetProvider returns an authenticated ProviderClient.
	GetProvider(ctx context.Context) (*golangsdk.ProviderClient, error)
	// GetServiceClient returns a ServiceClient for the given service type
	// (one of the Service* constants, e.g. ServiceELBv3). The client is
	// built via the SDK constructors so per-service endpoint fixups apply.
	GetServiceClient(ctx context.Context, service string, opts golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error)
	// AuthMethod returns the authentication method name (e.g., "password", "aksk").
	AuthMethod() string
	// Region returns the configured cloud region.
	Region() string
}

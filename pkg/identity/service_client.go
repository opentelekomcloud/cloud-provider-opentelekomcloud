package identity

import (
	"fmt"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"
)

// Service types accepted by IdentityProvider.GetServiceClient.
const (
	ServiceELBv3     = "elbv3"
	ServiceNetworkV1 = "network"
)

// newServiceClient delegates to the SDK constructors, which apply the
// per-service endpoint fixups a raw endpoint lookup would miss.
func newServiceClient(provider *golangsdk.ProviderClient, service string, opts golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error) {
	switch service {
	case ServiceELBv3:
		return openstack.NewELBV3(provider, opts)
	case ServiceNetworkV1:
		return openstack.NewNetworkV1(provider, opts)
	default:
		return nil, fmt.Errorf("unsupported service type %q", service)
	}
}

package identity

import (
	"testing"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
)

func fakeProvider(endpoint string) *golangsdk.ProviderClient {
	return &golangsdk.ProviderClient{
		EndpointLocator: func(golangsdk.EndpointOpts) (string, error) {
			return endpoint, nil
		},
	}
}

func TestNewServiceClient_ELBv3(t *testing.T) {
	endpoint := "https://elb.eu-de.otc.t-systems.com/v3/project-id/"
	sc, err := newServiceClient(fakeProvider(endpoint), ServiceELBv3, golangsdk.EndpointOpts{Region: "eu-de"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sc.Endpoint != endpoint {
		t.Errorf("Endpoint = %q, want %q", sc.Endpoint, endpoint)
	}
	// The SDK constructor must apply the ELBv3 fixup; without it all
	// resource URLs would miss the "elb/" path segment.
	if want := endpoint + "elb/"; sc.ResourceBase != want {
		t.Errorf("ResourceBase = %q, want %q", sc.ResourceBase, want)
	}
}

func TestNewServiceClient_UnknownService(t *testing.T) {
	_, err := newServiceClient(fakeProvider("https://example.com/"), "no-such-service", golangsdk.EndpointOpts{})
	if err == nil {
		t.Fatal("expected error for unknown service type")
	}
}

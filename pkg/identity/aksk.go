package identity

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

// akskProvider authenticates to OTC via AK/SK (HMAC-SHA256 request signing).
type akskProvider struct {
	opts   config.AuthOpts
	client *golangsdk.ProviderClient
	mu     sync.Mutex
}

// NewAKSKProvider creates an IdentityProvider that authenticates using AK/SK credentials.
func NewAKSKProvider(opts config.AuthOpts) (IdentityProvider, error) {
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, fmt.Errorf("access-key and secret-key are required for AK/SK auth")
	}
	if opts.AuthURL == "" {
		return nil, fmt.Errorf("auth-url is required")
	}
	return &akskProvider{opts: opts}, nil
}

func (p *akskProvider) authenticate() (*golangsdk.ProviderClient, error) {
	client, err := openstack.NewClient(p.opts.AuthURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenStack client: %w", err)
	}

	transport, err := buildTransport(p.opts)
	if err != nil {
		return nil, err
	}
	client.HTTPClient = http.Client{Transport: transport}
	client.UserAgent.Prepend("cloud-provider-opentelekomcloud")

	akskOpts := golangsdk.AKSKAuthOptions{
		IdentityEndpoint: p.opts.AuthURL,
		AccessKey:        p.opts.AccessKey,
		SecretKey:        p.opts.SecretKey,
		SecurityToken:    p.opts.SecurityToken,
		ProjectId:        p.opts.ProjectID,
		ProjectName:      p.opts.TenantName,
		Region:           p.opts.Region,
		Domain:           p.opts.DomainName,
		DomainID:         p.opts.DomainID,
	}

	if err := openstack.Authenticate(client, akskOpts); err != nil {
		return nil, fmt.Errorf("AK/SK authentication failed: %w", err)
	}
	return client, nil
}

func (p *akskProvider) GetProvider(_ context.Context) (*golangsdk.ProviderClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.client != nil {
		return p.client, nil
	}

	client, err := p.authenticate()
	if err != nil {
		return nil, err
	}
	p.client = client
	return p.client, nil
}

func (p *akskProvider) GetServiceClient(ctx context.Context, _ string, opts golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error) {
	provider, err := p.GetProvider(ctx)
	if err != nil {
		return nil, err
	}

	endpoint, err := provider.EndpointLocator(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to locate endpoint: %w", err)
	}

	return &golangsdk.ServiceClient{
		ProviderClient: provider,
		Endpoint:       endpoint,
	}, nil
}

func (p *akskProvider) AuthMethod() string {
	return config.AuthMethodAKSK
}

func (p *akskProvider) Region() string {
	return p.opts.Region
}

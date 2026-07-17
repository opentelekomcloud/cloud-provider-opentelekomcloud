package identity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

// passwordProvider authenticates to OTC via username/password (Keystone v3).
type passwordProvider struct {
	opts   config.AuthOpts
	client *golangsdk.ProviderClient
	mu     sync.Mutex
}

// NewPasswordProvider creates an IdentityProvider that authenticates using username/password.
func NewPasswordProvider(opts config.AuthOpts) (IdentityProvider, error) {
	if opts.Username == "" || opts.Password == "" {
		return nil, fmt.Errorf("username and password are required for password auth")
	}
	if opts.AuthURL == "" {
		return nil, fmt.Errorf("auth-url is required")
	}
	return &passwordProvider{opts: opts}, nil
}

func (p *passwordProvider) authenticate() (*golangsdk.ProviderClient, error) {
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

	authOpts := golangsdk.AuthOptions{
		IdentityEndpoint: p.opts.AuthURL,
		Username:         p.opts.Username,
		Password:         p.opts.Password,
		DomainName:       p.opts.DomainName,
		DomainID:         p.opts.DomainID,
		TenantID:         p.opts.TenantID,
		TenantName:       p.opts.TenantName,
		AllowReauth:      true,
	}

	if err := openstack.Authenticate(client, authOpts); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return client, nil
}

func (p *passwordProvider) GetProvider(_ context.Context) (*golangsdk.ProviderClient, error) {
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

func (p *passwordProvider) GetServiceClient(ctx context.Context, service string, opts golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error) {
	provider, err := p.GetProvider(ctx)
	if err != nil {
		return nil, err
	}
	return newServiceClient(provider, service, opts)
}

func (p *passwordProvider) AuthMethod() string {
	return config.AuthMethodPassword
}

func (p *passwordProvider) Region() string {
	return p.opts.Region
}

// buildTransport creates an http.Transport with optional TLS configuration.
func buildTransport(opts config.AuthOpts) (*http.Transport, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if opts.TLSInsecure {
		tlsConfig.InsecureSkipVerify = true // #nosec G402
	}

	if opts.CAFile != "" {
		caCert, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file %s: %w", opts.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", opts.CAFile)
		}
		tlsConfig.RootCAs = pool
	}

	return &http.Transport{TLSClientConfig: tlsConfig}, nil
}

package identity

import (
	"context"
	"time"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"k8s.io/klog/v2"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

const defaultRefreshInterval = 23 * time.Hour

// tokenCacheProvider wraps an IdentityProvider and proactively refreshes
// the authentication token before it expires. For AK/SK providers this is
// a no-op passthrough since each request is individually signed.
type tokenCacheProvider struct {
	inner    IdentityProvider
	interval time.Duration
}

// WithTokenCache wraps an IdentityProvider with proactive token refresh.
// For AK/SK auth this returns the original provider unchanged (no tokens to refresh).
// The stop channel is used to terminate the background refresh goroutine.
// If interval is nil, the default refresh interval (23h) is used.
func WithTokenCache(provider IdentityProvider, interval *time.Duration, stop <-chan struct{}) IdentityProvider {
	if provider.AuthMethod() == config.AuthMethodAKSK {
		return provider
	}

	refreshInterval := defaultRefreshInterval
	if interval != nil {
		refreshInterval = *interval
	}

	cached := &tokenCacheProvider{
		inner:    provider,
		interval: refreshInterval,
	}

	go cached.refreshLoop(stop)

	return cached
}

func (c *tokenCacheProvider) refreshLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			klog.Info("Token refresh goroutine stopped")
			return
		case <-ticker.C:
			klog.V(4).Info("Proactively refreshing authentication token")
			provider, err := c.inner.GetProvider(context.Background())
			if err != nil {
				klog.Warningf("Failed to get provider for token refresh: %v", err)
				continue
			}
			if provider.ReauthFunc != nil {
				if err := provider.ReauthFunc(); err != nil {
					klog.Warningf("Proactive token refresh failed: %v", err)
				} else {
					klog.V(4).Info("Proactive token refresh succeeded")
				}
			}
		}
	}
}

func (c *tokenCacheProvider) GetProvider(ctx context.Context) (*golangsdk.ProviderClient, error) {
	return c.inner.GetProvider(ctx)
}

func (c *tokenCacheProvider) GetServiceClient(ctx context.Context, service string, opts golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error) {
	return c.inner.GetServiceClient(ctx, service, opts)
}

func (c *tokenCacheProvider) AuthMethod() string {
	return c.inner.AuthMethod()
}

func (c *tokenCacheProvider) Region() string {
	return c.inner.Region()
}

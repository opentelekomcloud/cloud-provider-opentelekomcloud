package identity

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

type mockIdentityProvider struct {
	authMethod string
	region     string
	client     *golangsdk.ProviderClient
}

func (m *mockIdentityProvider) GetProvider(_ context.Context) (*golangsdk.ProviderClient, error) {
	return m.client, nil
}

func (m *mockIdentityProvider) GetServiceClient(_ context.Context, _ string, _ golangsdk.EndpointOpts) (*golangsdk.ServiceClient, error) {
	return &golangsdk.ServiceClient{ProviderClient: m.client}, nil
}

func (m *mockIdentityProvider) AuthMethod() string { return m.authMethod }
func (m *mockIdentityProvider) Region() string     { return m.region }

func TestWithTokenCache_AKSKPassthrough(t *testing.T) {
	mock := &mockIdentityProvider{
		authMethod: config.AuthMethodAKSK,
		region:     "eu-de",
	}

	stop := make(chan struct{})
	defer close(stop)

	result := WithTokenCache(mock, nil, stop)

	// For AK/SK, WithTokenCache should return the original provider
	if result != mock {
		t.Error("expected AK/SK provider to be returned unchanged")
	}
}

func TestWithTokenCache_PasswordWrapped(t *testing.T) {
	mock := &mockIdentityProvider{
		authMethod: config.AuthMethodPassword,
		region:     "eu-de",
		client:     &golangsdk.ProviderClient{},
	}

	stop := make(chan struct{})
	defer close(stop)

	result := WithTokenCache(mock, nil, stop)

	if result == mock {
		t.Error("expected password provider to be wrapped")
	}
	if result.AuthMethod() != config.AuthMethodPassword {
		t.Errorf("AuthMethod() = %q, want %q", result.AuthMethod(), config.AuthMethodPassword)
	}
	if result.Region() != "eu-de" {
		t.Errorf("Region() = %q, want %q", result.Region(), "eu-de")
	}
}

func TestWithTokenCache_RefreshFires(t *testing.T) {
	var refreshCount int64

	client := &golangsdk.ProviderClient{}
	client.ReauthFunc = func() error {
		atomic.AddInt64(&refreshCount, 1)
		return nil
	}

	mock := &mockIdentityProvider{
		authMethod: config.AuthMethodPassword,
		region:     "eu-de",
		client:     client,
	}

	stop := make(chan struct{})
	interval := 50 * time.Millisecond
	_ = WithTokenCache(mock, &interval, stop)

	// Wait for at least one refresh to happen
	time.Sleep(200 * time.Millisecond)
	close(stop)

	count := atomic.LoadInt64(&refreshCount)
	if count == 0 {
		t.Error("expected at least one proactive refresh to fire")
	}
}

func TestWithTokenCache_StopChannel(t *testing.T) {
	var refreshCount int64

	client := &golangsdk.ProviderClient{}
	client.ReauthFunc = func() error {
		atomic.AddInt64(&refreshCount, 1)
		return nil
	}

	mock := &mockIdentityProvider{
		authMethod: config.AuthMethodPassword,
		region:     "eu-de",
		client:     client,
	}

	stop := make(chan struct{})
	interval := 50 * time.Millisecond
	_ = WithTokenCache(mock, &interval, stop)

	// Stop immediately
	close(stop)
	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt64(&refreshCount)
	// Should have 0 or at most 1 refresh (race between ticker and stop)
	if count > 1 {
		t.Errorf("expected at most 1 refresh after stop, got %d", count)
	}
}

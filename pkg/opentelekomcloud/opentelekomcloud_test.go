package opentelekomcloud

import (
	"strings"
	"testing"

	cloudprovider "k8s.io/cloud-provider"
)

const testConfig = `[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=test-ak
secret-key=test-sk
project-id=test-project
region=eu-de
domain-name=test-domain
`

func TestProviderRegistered(t *testing.T) {
	cfg := strings.NewReader(testConfig)
	provider, err := cloudprovider.GetCloudProvider(ProviderName, cfg)
	if err != nil {
		t.Fatalf("provider %q not registered: %v", ProviderName, err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestNewOTC(t *testing.T) {
	cfg := strings.NewReader(testConfig)
	cp, err := NewOTC(cfg)
	if err != nil {
		t.Fatalf("NewOTC returned error: %v", err)
	}
	if cp == nil {
		t.Fatal("NewOTC returned nil")
	}
	if cp.ProviderName() != ProviderName {
		t.Errorf("expected provider name %q, got %q", ProviderName, cp.ProviderName())
	}
	if !cp.HasClusterID() {
		t.Error("expected HasClusterID to return true")
	}
}

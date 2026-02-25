package opentelekomcloud

import (
	"testing"

	cloudprovider "k8s.io/cloud-provider"
)

func TestProviderRegistered(t *testing.T) {
	provider, err := cloudprovider.GetCloudProvider(ProviderName, nil)
	if err != nil {
		t.Fatalf("provider %q not registered: %v", ProviderName, err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestNewOTC(t *testing.T) {
	cp, err := NewOTC(nil)
	if err != nil {
		t.Fatalf("NewOTC returned error: %v", err)
	}
	if cp == nil {
		t.Fatal("NewOTC returned nil")
	}
}

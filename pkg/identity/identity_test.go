package identity

import (
	"testing"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

func TestNewIdentityProvider_AKSK(t *testing.T) {
	opts := config.AuthOpts{
		AuthURL:   "https://iam.eu-de.otc.t-systems.com/v3",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:    "eu-de",
	}

	provider, err := NewIdentityProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.AuthMethod() != config.AuthMethodAKSK {
		t.Errorf("AuthMethod() = %q, want %q", provider.AuthMethod(), config.AuthMethodAKSK)
	}
	if provider.Region() != "eu-de" {
		t.Errorf("Region() = %q, want %q", provider.Region(), "eu-de")
	}
}

func TestNewIdentityProvider_Password(t *testing.T) {
	opts := config.AuthOpts{
		AuthURL:    "https://iam.eu-de.otc.t-systems.com/v3",
		Username:   "user",
		Password:   "pass",
		DomainName: "domain",
		Region:     "eu-de",
	}

	provider, err := NewIdentityProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.AuthMethod() != config.AuthMethodPassword {
		t.Errorf("AuthMethod() = %q, want %q", provider.AuthMethod(), config.AuthMethodPassword)
	}
}

func TestNewIdentityProvider_AKSKPriority(t *testing.T) {
	// When both credential types are present, AK/SK should be chosen
	opts := config.AuthOpts{
		AuthURL:   "https://iam.eu-de.otc.t-systems.com/v3",
		Username:  "user",
		Password:  "pass",
		AccessKey: "ak",
		SecretKey: "sk",
		Region:    "eu-de",
	}

	provider, err := NewIdentityProvider(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.AuthMethod() != config.AuthMethodAKSK {
		t.Errorf("AuthMethod() = %q, want %q (AK/SK should take priority)", provider.AuthMethod(), config.AuthMethodAKSK)
	}
}

func TestNewIdentityProvider_NoCredentials(t *testing.T) {
	opts := config.AuthOpts{
		AuthURL: "https://iam.eu-de.otc.t-systems.com/v3",
	}

	_, err := NewIdentityProvider(opts)
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

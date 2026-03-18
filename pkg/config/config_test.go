package config

import (
	"strings"
	"testing"
)

const passwordConfig = `
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
username=my-user
password=my-password
domain-name=my-domain
tenant-name=eu-de_project
region=eu-de
`

const akskConfig = `
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=AKIAIOSFODNN7EXAMPLE
secret-key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
project-id=abc123
region=eu-de
domain-name=my-domain
`

func TestReadConfig_Password(t *testing.T) {
	cc, err := ReadConfig(strings.NewReader(passwordConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cc.Global.AuthURL != "https://iam.eu-de.otc.t-systems.com/v3" {
		t.Errorf("AuthURL = %q, want %q", cc.Global.AuthURL, "https://iam.eu-de.otc.t-systems.com/v3")
	}
	if cc.Global.Username != "my-user" {
		t.Errorf("Username = %q, want %q", cc.Global.Username, "my-user")
	}
	if cc.Global.Password != "my-password" {
		t.Errorf("Password = %q, want %q", cc.Global.Password, "my-password")
	}
	if cc.Global.AuthMethod() != AuthMethodPassword {
		t.Errorf("AuthMethod() = %q, want %q", cc.Global.AuthMethod(), AuthMethodPassword)
	}
}

func TestReadConfig_AKSK(t *testing.T) {
	cc, err := ReadConfig(strings.NewReader(akskConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cc.Global.AccessKey != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("AccessKey = %q, want %q", cc.Global.AccessKey, "AKIAIOSFODNN7EXAMPLE")
	}
	if cc.Global.SecretKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("SecretKey = %q, want %q", cc.Global.SecretKey, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	}
	if cc.Global.AuthMethod() != AuthMethodAKSK {
		t.Errorf("AuthMethod() = %q, want %q", cc.Global.AuthMethod(), AuthMethodAKSK)
	}
}

func TestReadConfig_EnvFallback(t *testing.T) {
	t.Setenv("OS_AUTH_URL", "https://iam.eu-de.otc.t-systems.com/v3")
	t.Setenv("OS_ACCESS_KEY", "env-ak")
	t.Setenv("OS_SECRET_KEY", "env-sk")
	t.Setenv("OS_REGION_NAME", "eu-de")

	cc, err := ReadConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cc.Global.AuthURL != "https://iam.eu-de.otc.t-systems.com/v3" {
		t.Errorf("AuthURL = %q, want env value", cc.Global.AuthURL)
	}
	if cc.Global.AccessKey != "env-ak" {
		t.Errorf("AccessKey = %q, want %q", cc.Global.AccessKey, "env-ak")
	}
	if cc.Global.Region != "eu-de" {
		t.Errorf("Region = %q, want %q", cc.Global.Region, "eu-de")
	}
}

func TestReadConfig_ConfigOverridesEnv(t *testing.T) {
	t.Setenv("OS_AUTH_URL", "https://env-url/v3")
	t.Setenv("OS_ACCESS_KEY", "env-ak")
	t.Setenv("OS_SECRET_KEY", "env-sk")

	cc, err := ReadConfig(strings.NewReader(akskConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cc.Global.AuthURL != "https://iam.eu-de.otc.t-systems.com/v3" {
		t.Errorf("AuthURL = %q, want config value not env", cc.Global.AuthURL)
	}
	if cc.Global.AccessKey != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("AccessKey = %q, want config value not env", cc.Global.AccessKey)
	}
}

func TestReadConfig_MissingAuthURL(t *testing.T) {
	cfg := `
[Global]
username=user
password=pass
`
	_, err := ReadConfig(strings.NewReader(cfg))
	if err == nil {
		t.Fatal("expected error for missing auth-url")
	}
	if !strings.Contains(err.Error(), "auth-url") {
		t.Errorf("error = %q, want mention of auth-url", err.Error())
	}
}

func TestReadConfig_NoCredentials(t *testing.T) {
	cfg := `
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
`
	_, err := ReadConfig(strings.NewReader(cfg))
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Errorf("error = %q, want mention of credentials", err.Error())
	}
}

func TestAuthOpts_AuthMethod_Priority(t *testing.T) {
	// When both are set, AK/SK takes priority
	opts := AuthOpts{
		Username:  "user",
		Password:  "pass",
		AccessKey: "ak",
		SecretKey: "sk",
	}
	if opts.AuthMethod() != AuthMethodAKSK {
		t.Errorf("AuthMethod() = %q, want %q (AK/SK should take priority)", opts.AuthMethod(), AuthMethodAKSK)
	}
}

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
access-key=test-access-key
secret-key=test-secret-key
project-id=abc123
region=eu-de
domain-name=my-domain
`

// clearAuthEnv blanks every variable consumed by applyEnvFallbacks so ambient
// credentials can neither change results nor be printed by failure messages.
func clearAuthEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OS_AUTH_URL", "OS_USERNAME", "OS_PASSWORD",
		"OS_ACCESS_KEY", "OS_SECRET_KEY", "OS_SECURITY_TOKEN",
		"OS_DOMAIN_NAME", "OS_DOMAIN_ID", "OS_TENANT_ID",
		"OS_TENANT_NAME", "OS_PROJECT_ID", "OS_REGION_NAME",
	} {
		t.Setenv(key, "")
	}
}

func TestReadConfig_Password(t *testing.T) {
	clearAuthEnv(t)
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
		t.Error("Password mismatch")
	}
	if cc.Global.AuthMethod() != AuthMethodPassword {
		t.Errorf("AuthMethod() = %q, want %q", cc.Global.AuthMethod(), AuthMethodPassword)
	}
}

func TestReadConfig_AKSK(t *testing.T) {
	clearAuthEnv(t)
	cc, err := ReadConfig(strings.NewReader(akskConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cc.Global.AccessKey != "test-access-key" {
		t.Errorf("AccessKey = %q, want %q", cc.Global.AccessKey, "test-access-key")
	}
	if cc.Global.SecretKey != "test-secret-key" {
		t.Error("SecretKey mismatch")
	}
	if cc.Global.AuthMethod() != AuthMethodAKSK {
		t.Errorf("AuthMethod() = %q, want %q", cc.Global.AuthMethod(), AuthMethodAKSK)
	}
}

func TestReadConfig_EnvFallback(t *testing.T) {
	clearAuthEnv(t)
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
	clearAuthEnv(t)
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
	if cc.Global.AccessKey != "test-access-key" {
		t.Errorf("AccessKey = %q, want config value not env", cc.Global.AccessKey)
	}
}

func TestReadConfig_MissingAuthURL(t *testing.T) {
	clearAuthEnv(t)
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
	clearAuthEnv(t)
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

const lbConfig = `
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=ak
secret-key=sk
region=eu-de

[LoadBalancer]
subnet-id=subnet-1234
vpc-id=vpc-1234
availability-zone=eu-de-01
availability-zone=eu-de-02
l4-flavor-id=flavor-1
lb-algorithm=LEAST_CONNECTIONS
health-check-enabled=false
health-check-delay=10
`

func TestReadConfig_LoadBalancer(t *testing.T) {
	cc, err := ReadConfig(strings.NewReader(lbConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lb := cc.LoadBalancer
	if lb.SubnetID != "subnet-1234" {
		t.Errorf("SubnetID = %q, want %q", lb.SubnetID, "subnet-1234")
	}
	if lb.VpcID != "vpc-1234" {
		t.Errorf("VpcID = %q, want %q", lb.VpcID, "vpc-1234")
	}
	if len(lb.AvailabilityZones) != 2 || lb.AvailabilityZones[0] != "eu-de-01" || lb.AvailabilityZones[1] != "eu-de-02" {
		t.Errorf("AvailabilityZones = %v, want [eu-de-01 eu-de-02]", lb.AvailabilityZones)
	}
	if lb.L4FlavorID != "flavor-1" {
		t.Errorf("L4FlavorID = %q, want %q", lb.L4FlavorID, "flavor-1")
	}
	if lb.LBAlgorithm != "LEAST_CONNECTIONS" {
		t.Errorf("LBAlgorithm = %q, want %q", lb.LBAlgorithm, "LEAST_CONNECTIONS")
	}
	if lb.HealthCheckEnabled {
		t.Error("HealthCheckEnabled = true, want false")
	}
	if lb.HealthCheckDelay != 10 {
		t.Errorf("HealthCheckDelay = %d, want 10", lb.HealthCheckDelay)
	}
	// Untouched keys keep their defaults.
	if lb.HealthCheckTimeout != 3 || lb.HealthCheckMaxRetries != 3 {
		t.Errorf("health check defaults not preserved: timeout=%d retries=%d", lb.HealthCheckTimeout, lb.HealthCheckMaxRetries)
	}
}

func TestReadConfig_LoadBalancerDefaults(t *testing.T) {
	cc, err := ReadConfig(strings.NewReader(akskConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lb := cc.LoadBalancer
	if lb.LBAlgorithm != "ROUND_ROBIN" || !lb.HealthCheckEnabled || lb.HealthCheckDelay != 5 {
		t.Errorf("unexpected defaults: %+v", lb)
	}
}

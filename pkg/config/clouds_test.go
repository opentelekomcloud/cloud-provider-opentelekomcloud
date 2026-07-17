package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testCloudsYAML = `clouds:
  otc-password:
    region_name: eu-de
    auth:
      auth_url: https://iam.eu-de.otc.t-systems.com/v3
      username: my-user
      password: my-password
      domain_name: OTC000000001
      project_name: eu-de_project
  otc-aksk:
    region_name: eu-nl
    auth:
      auth_url: https://iam.eu-nl.otc.t-systems.com/v3
      ak: my-access-key
      sk: my-secret-key
      project_id: pid-123
      domain_name: OTC000000001
`

func withCloudsYAML(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "clouds.yaml")
	if err := os.WriteFile(path, []byte(testCloudsYAML), 0o600); err != nil {
		t.Fatalf("failed to write clouds.yaml: %v", err)
	}
	t.Setenv("OS_CLIENT_CONFIG_FILE", path)
	// Neutralize ambient variables that the SDK env loader (openstack.NewEnv,
	// gophertelekomcloud loader.go/auth_env.go) would otherwise merge into the
	// result — including every AK/SK and security-token alias, so a real
	// credential from the developer's or CI's environment can never end up in
	// the parsed AuthOpts (or in test output on failure).
	for _, key := range []string{
		"OS_CLOUD", "OS_AUTH_URL", "OS_USERNAME", "OS_PASSWORD",
		"OS_ACCESS_KEY", "OS_SECRET_KEY", "OS_DOMAIN_NAME", "OS_PROJECT_ID",
		"OS_PROJECT_NAME", "OS_TENANT_NAME", "OS_REGION_NAME",
		"OS_AK", "OS_SK", "OS_ACCESS_KEY_ID", "OS_ACCESS_KEY_SECRET",
		"AWS_ACCESS_KEY_ID", "AWS_ACCESS_SECRET_KEY",
		"OS_SECURITY_TOKEN", "OS_AKSK_SECURITY_TOKEN", "OS_ST", "AWS_SECURITY_TOKEN",
		"OS_TOKEN", "OS_TOKEN_ID", "OS_PASSCODE",
		"OS_DOMAIN_ID", "OS_TENANT_ID", "OS_USER_DOMAIN_NAME", "OS_USER_DOMAIN_ID",
	} {
		t.Setenv(key, "")
	}
}

func TestAuthOptsFromCloudsYAML_Password(t *testing.T) {
	withCloudsYAML(t)

	opts, err := AuthOptsFromCloudsYAML("otc-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.AuthMethod() != AuthMethodPassword {
		t.Errorf("AuthMethod = %q, want %q", opts.AuthMethod(), AuthMethodPassword)
	}
	if opts.Username != "my-user" || opts.Password != "my-password" {
		t.Errorf("unexpected credentials: %+v", opts)
	}
	if opts.DomainName != "OTC000000001" || opts.TenantName != "eu-de_project" {
		t.Errorf("unexpected scope: domain=%q tenant=%q", opts.DomainName, opts.TenantName)
	}
	if opts.Region != "eu-de" {
		t.Errorf("Region = %q, want eu-de", opts.Region)
	}
}

func TestAuthOptsFromCloudsYAML_AKSK(t *testing.T) {
	withCloudsYAML(t)

	opts, err := AuthOptsFromCloudsYAML("otc-aksk")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.AuthMethod() != AuthMethodAKSK {
		t.Errorf("AuthMethod = %q, want %q", opts.AuthMethod(), AuthMethodAKSK)
	}
	if opts.AccessKey != "my-access-key" || opts.SecretKey != "my-secret-key" {
		t.Errorf("unexpected credentials: %+v", opts)
	}
	if opts.ProjectID != "pid-123" || opts.Region != "eu-nl" {
		t.Errorf("unexpected scope: project=%q region=%q", opts.ProjectID, opts.Region)
	}
}

func TestAuthOptsFromCloudsYAML_UnknownCloud(t *testing.T) {
	withCloudsYAML(t)

	if _, err := AuthOptsFromCloudsYAML("no-such-cloud"); err == nil {
		t.Error("expected error for unknown cloud entry")
	}
}

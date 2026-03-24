package config

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/gcfg.v1"
)

const (
	AuthMethodPassword = "password"
	AuthMethodAKSK     = "aksk"
)

// CloudConfig holds the cloud provider configuration parsed from an INI file.
type CloudConfig struct {
	Global AuthOpts
}

// AuthOpts contains authentication and connection options for OTC.
type AuthOpts struct {
	// AuthURL is the Identity (IAM) endpoint URL.
	AuthURL string `gcfg:"auth-url"`

	// Password auth
	Username string `gcfg:"username"`
	Password string `gcfg:"password"`

	// AK/SK auth
	AccessKey     string `gcfg:"access-key"`
	SecretKey     string `gcfg:"secret-key"`
	SecurityToken string `gcfg:"security-token"`

	// Scope
	DomainName string `gcfg:"domain-name"`
	DomainID   string `gcfg:"domain-id"`
	TenantID   string `gcfg:"tenant-id"`
	TenantName string `gcfg:"tenant-name"`
	ProjectID  string `gcfg:"project-id"`
	Region     string `gcfg:"region"`

	// TLS
	CAFile      string `gcfg:"ca-file"`
	TLSInsecure bool   `gcfg:"tls-insecure"`
}

// AuthMethod returns the authentication method based on which credentials are set.
// Priority: AK/SK > Password.
func (o *AuthOpts) AuthMethod() string {
	if o.AccessKey != "" && o.SecretKey != "" {
		return AuthMethodAKSK
	}
	if o.Username != "" && o.Password != "" {
		return AuthMethodPassword
	}
	return ""
}

// ReadConfig parses the cloud configuration from an INI reader, then applies
// environment variable fallbacks for any unset fields.
func ReadConfig(cfg io.Reader) (*CloudConfig, error) {
	cc := &CloudConfig{}

	if cfg != nil {
		if err := gcfg.FatalOnly(gcfg.ReadInto(cc, cfg)); err != nil {
			return nil, fmt.Errorf("failed to parse cloud config: %w", err)
		}
	}

	applyEnvFallbacks(&cc.Global)

	if err := validate(&cc.Global); err != nil {
		return nil, err
	}

	return cc, nil
}

// applyEnvFallbacks sets AuthOpts fields from environment variables when
// the field is not already set by the config file.
func applyEnvFallbacks(opts *AuthOpts) {
	setIfEmpty(&opts.AuthURL, "OS_AUTH_URL")
	setIfEmpty(&opts.Username, "OS_USERNAME")
	setIfEmpty(&opts.Password, "OS_PASSWORD")
	setIfEmpty(&opts.AccessKey, "OS_ACCESS_KEY")
	setIfEmpty(&opts.SecretKey, "OS_SECRET_KEY")
	setIfEmpty(&opts.SecurityToken, "OS_SECURITY_TOKEN")
	setIfEmpty(&opts.DomainName, "OS_DOMAIN_NAME")
	setIfEmpty(&opts.DomainID, "OS_DOMAIN_ID")
	setIfEmpty(&opts.TenantID, "OS_TENANT_ID")
	setIfEmpty(&opts.TenantName, "OS_TENANT_NAME")
	setIfEmpty(&opts.ProjectID, "OS_PROJECT_ID")
	setIfEmpty(&opts.Region, "OS_REGION_NAME")
}

func setIfEmpty(field *string, envVar string) {
	if *field == "" {
		*field = os.Getenv(envVar)
	}
}

// validate checks that the config has the minimum required fields.
func validate(opts *AuthOpts) error {
	if opts.AuthURL == "" {
		return fmt.Errorf("auth-url is required (set in config or OS_AUTH_URL)")
	}
	if opts.AuthMethod() == "" {
		return fmt.Errorf("no valid credentials: provide username/password or access-key/secret-key")
	}
	return nil
}

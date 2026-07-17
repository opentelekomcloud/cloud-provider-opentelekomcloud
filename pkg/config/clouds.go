package config

import (
	"fmt"

	"github.com/opentelekomcloud/gophertelekomcloud/openstack"
)

// AuthOptsFromCloudsYAML loads credentials from a standard clouds.yaml
// merged with OS_* env vars; the entry is chosen by name or OS_CLOUD.
// Convenience for CLI/test use – the manager itself uses ReadConfig.
func AuthOptsFromCloudsYAML(cloudName string) (*AuthOpts, error) {
	env := openstack.NewEnv("OS_")
	var (
		cloud *openstack.Cloud
		err   error
	)
	if cloudName != "" {
		cloud, err = env.Cloud(cloudName)
	} else {
		cloud, err = env.Cloud()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load clouds.yaml: %w", err)
	}

	auth := cloud.AuthInfo
	opts := &AuthOpts{
		AuthURL:       auth.AuthURL,
		Username:      auth.Username,
		Password:      auth.Password,
		AccessKey:     auth.AccessKey,
		SecretKey:     auth.SecretKey,
		SecurityToken: auth.SecurityToken,
		DomainName:    auth.DomainName,
		DomainID:      auth.DomainID,
		TenantName:    auth.ProjectName,
		ProjectID:     auth.ProjectID,
		Region:        cloud.RegionName,
	}
	if opts.DomainName == "" {
		opts.DomainName = auth.UserDomainName
	}
	if opts.DomainID == "" {
		opts.DomainID = auth.UserDomainID
	}
	if opts.Region == "" && len(cloud.Regions) > 0 {
		opts.Region = cloud.Regions[0]
	}
	// Password-auth scoping uses tenant fields; prefer the project name and
	// fall back to the project ID to avoid sending both.
	if opts.TenantName == "" && auth.ProjectID != "" {
		opts.TenantID = auth.ProjectID
	}

	if err := validate(opts); err != nil {
		return nil, fmt.Errorf("clouds.yaml cloud %q: %w", cloud.Cloud, err)
	}
	return opts, nil
}

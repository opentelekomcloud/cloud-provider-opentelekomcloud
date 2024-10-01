package opentelekomcloud

import (
	"fmt"
	"gopkg.in/gcfg.v1"
	"io"
	"k8s.io/cloud-provider/options"
)

type CloudConfig struct {
	VpcID           string `gcfg:"vpc_id"`
	SubnetID        string `gcfg:"subnet_id"`
	SecurityGroupID string `gcfg:"security_group_id"`
}

func ReadConfig(cfg io.Reader) (*CloudConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("must provide a config file")
	}

	cc := &CloudConfig{}
	// Read configuration
	err := gcfg.FatalOnly(gcfg.ReadInto(cc, cfg))
	if err != nil {
		return nil, err
	}

	return cc, nil
}

type CloudProvider struct {
	cloudControllerManagerOpts *options.CloudControllerManagerOptions
}

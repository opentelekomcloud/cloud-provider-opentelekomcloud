package opentelekomcloud

import (
	"io"
	cloudprovider "k8s.io/cloud-provider"
)

const (
	ProviderName = "opentelekomcloud"
)

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		otc, err := NewOTC(config)
		if err != nil {
			return nil, err
		}
		return otc, nil
	})
}

type CloudProvider struct {
	cloudprovider.Interface
}

func NewOTC(cfg io.Reader) (*CloudProvider, error) {
	return &CloudProvider{}, nil
}

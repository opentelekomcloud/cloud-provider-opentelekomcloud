package opentelekomcloud

import (
	"io"

	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/identity"
)

const (
	ProviderName = "opentelekomcloud"
)

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(cfg io.Reader) (cloudprovider.Interface, error) {
		otc, err := NewOTC(cfg)
		if err != nil {
			return nil, err
		}
		return otc, nil
	})
}

type CloudProvider struct {
	cloudprovider.Interface
	identity identity.IdentityProvider
	config   *config.CloudConfig
}

func NewOTC(cfg io.Reader) (*CloudProvider, error) {
	cc, err := config.ReadConfig(cfg)
	if err != nil {
		return nil, err
	}

	idProvider, err := identity.NewIdentityProvider(cc.Global)
	if err != nil {
		return nil, err
	}

	klog.Infof("OTC cloud provider initialized with auth method: %s", idProvider.AuthMethod())

	return &CloudProvider{
		identity: idProvider,
		config:   cc,
	}, nil
}

func (cp *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	cp.identity = identity.WithTokenCache(cp.identity, nil, stop)
	klog.Info("OTC cloud provider token cache initialized")
}

func (cp *CloudProvider) ProviderName() string {
	return ProviderName
}

func (cp *CloudProvider) HasClusterID() bool {
	return true
}

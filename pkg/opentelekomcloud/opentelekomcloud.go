package opentelekomcloud

import (
	"io"

	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/elb"
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

// CloudProvider implements cloudprovider.Interface for Open Telekom Cloud.
type CloudProvider struct {
	identity identity.IdentityProvider
	config   *config.CloudConfig
	lb       cloudprovider.LoadBalancer
}

var _ cloudprovider.Interface = &CloudProvider{}

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

	cp := &CloudProvider{
		identity: idProvider,
		config:   cc,
	}
	// The getter is evaluated per call, so the load balancer picks up the
	// token-cache wrapper installed by Initialize.
	cp.lb = elb.NewLoadBalancer(func() identity.IdentityProvider { return cp.identity }, cc.LoadBalancer)
	return cp, nil
}

func (cp *CloudProvider) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	cp.identity = identity.WithTokenCache(cp.identity, nil, stop)
	klog.Info("OTC cloud provider token cache initialized")
}

// LoadBalancer returns the load balancer implementation backed by OTC ELBv3.
func (cp *CloudProvider) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return cp.lb, true
}

// Instances is not implemented yet.
func (cp *CloudProvider) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 is not implemented yet.
func (cp *CloudProvider) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return nil, false
}

// Zones is not implemented yet.
func (cp *CloudProvider) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Clusters is not supported.
func (cp *CloudProvider) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// Routes is not supported.
func (cp *CloudProvider) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (cp *CloudProvider) ProviderName() string {
	return ProviderName
}

func (cp *CloudProvider) HasClusterID() bool {
	return true
}

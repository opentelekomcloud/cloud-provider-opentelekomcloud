// Package elb implements the cloudprovider.LoadBalancer interface on top of
// the Open Telekom Cloud ELBv3 (dedicated load balancer) API.
package elb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/listeners"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/loadbalancers"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/members"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/monitors"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/elb/v3/pools"
	"github.com/opentelekomcloud/gophertelekomcloud/openstack/networking/v1/eips"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/identity"
)

const (
	// lbNamePrefix follows the OpenStack cloud provider naming convention.
	lbNamePrefix = "kube_service"

	maxResourceNameLength = 255

	statusActive = "ACTIVE"
	statusError  = "ERROR"

	activePollInterval = 3 * time.Second
	activePollTimeout  = 5 * time.Minute
)

// LoadBalancer manages OTC ELBv3 load balancers for Kubernetes Services.
type LoadBalancer struct {
	opts config.LoadBalancerOpts

	// newClient builds an ELBv3 service client. Overridable in tests.
	newClient func(ctx context.Context) (*golangsdk.ServiceClient, error)

	// newNetworkClient builds a VPC/network v1 service client (EIP API).
	// Overridable in tests.
	newNetworkClient func(ctx context.Context) (*golangsdk.ServiceClient, error)
}

// NewLoadBalancer creates a LoadBalancer. The identity getter is evaluated
// per call so a later token-cache wrapper is picked up.
func NewLoadBalancer(getIdentity func() identity.IdentityProvider, opts config.LoadBalancerOpts) *LoadBalancer {
	return &LoadBalancer{
		opts: opts,
		newClient: func(ctx context.Context) (*golangsdk.ServiceClient, error) {
			id := getIdentity()
			client, err := id.GetServiceClient(ctx, identity.ServiceELBv3, golangsdk.EndpointOpts{Region: id.Region()})
			if err != nil {
				return nil, fmt.Errorf("failed to create ELBv3 client: %w", err)
			}
			return client, nil
		},
		newNetworkClient: func(ctx context.Context) (*golangsdk.ServiceClient, error) {
			id := getIdentity()
			client, err := id.GetServiceClient(ctx, identity.ServiceNetworkV1, golangsdk.EndpointOpts{Region: id.Region()})
			if err != nil {
				return nil, fmt.Errorf("failed to create network client: %w", err)
			}
			return client, nil
		},
	}
}

// GetLoadBalancerName follows the OpenStack provider naming convention.
func (lb *LoadBalancer) GetLoadBalancerName(_ context.Context, clusterName string, service *v1.Service) string {
	name := fmt.Sprintf("%s_%s_%s_%s", lbNamePrefix, clusterName, service.Namespace, service.Name)
	return cutString(name, maxResourceNameLength)
}

// GetLoadBalancer returns the load balancer status if it exists.
func (lb *LoadBalancer) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	settings, err := settingsForService(lb.opts, service)
	if err != nil {
		return nil, false, err
	}
	client, err := lb.newClient(ctx)
	if err != nil {
		return nil, false, err
	}

	balancer, err := lb.findLoadBalancer(client, settings, lb.GetLoadBalancerName(ctx, clusterName, service))
	if err != nil {
		return nil, false, err
	}
	if balancer == nil {
		return nil, false, nil
	}
	return lbStatus(balancer), true, nil
}

// EnsureLoadBalancer reconciles the LB and all its sub-resources.
func (lb *LoadBalancer) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("no ports provided for load balancer service %s/%s", service.Namespace, service.Name)
	}
	if err := validatePorts(service); err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available for load balancer service %s/%s", service.Namespace, service.Name)
	}

	settings, err := settingsForService(lb.opts, service)
	if err != nil {
		return nil, err
	}
	client, err := lb.newClient(ctx)
	if err != nil {
		return nil, err
	}

	name := lb.GetLoadBalancerName(ctx, clusterName, service)
	balancer, err := lb.findLoadBalancer(client, settings, name)
	if err != nil {
		return nil, err
	}
	if balancer == nil {
		if settings.LoadBalancerID != "" {
			return nil, fmt.Errorf("load balancer %q referenced by annotation %s not found", settings.LoadBalancerID, AnnotationID)
		}
		balancer, err = lb.createLoadBalancer(client, settings, name, service)
		if err != nil {
			return nil, err
		}
		klog.V(2).Infof("Created load balancer %s (%s) for service %s/%s", balancer.Name, balancer.ID, service.Namespace, service.Name)
	}

	if err := waitForActive(ctx, client, balancer.ID); err != nil {
		return nil, err
	}

	if err := lb.reconcile(ctx, client, balancer, name, settings, service, nodes); err != nil {
		return nil, err
	}

	// Re-read for fresh EIP/VIP data.
	balancer, err = loadbalancers.Get(client, balancer.ID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get load balancer after reconcile: %w", err)
	}
	return lbStatus(balancer), nil
}

// UpdateLoadBalancer reconciles pool members after the node set changed.
func (lb *LoadBalancer) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	settings, err := settingsForService(lb.opts, service)
	if err != nil {
		return err
	}
	client, err := lb.newClient(ctx)
	if err != nil {
		return err
	}

	name := lb.GetLoadBalancerName(ctx, clusterName, service)
	balancer, err := lb.findLoadBalancer(client, settings, name)
	if err != nil {
		return err
	}
	if balancer == nil {
		return fmt.Errorf("load balancer for service %s/%s not found", service.Namespace, service.Name)
	}

	existing, err := listListeners(client, balancer.ID)
	if err != nil {
		return err
	}
	byKey := listenersByKey(existing)

	for _, port := range service.Spec.Ports {
		listener, ok := byKey[portKey(port)]
		if !ok {
			return fmt.Errorf("listener for port %s/%d of service %s/%s not found", port.Protocol, port.Port, service.Namespace, service.Name)
		}
		pool, err := findPoolForListener(client, listener.ID)
		if err != nil {
			return err
		}
		if pool == nil {
			return fmt.Errorf("pool for listener %s not found", listener.ID)
		}
		if err := lb.reconcileMembers(ctx, client, balancer, pool.ID, port.NodePort, nodes); err != nil {
			return err
		}
	}
	return nil
}

// EnsureLoadBalancerDeleted removes everything this Service owns; a shared
// LB (id annotation) itself is kept.
func (lb *LoadBalancer) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	settings, err := settingsForService(lb.opts, service)
	if err != nil {
		return err
	}
	client, err := lb.newClient(ctx)
	if err != nil {
		return err
	}

	name := lb.GetLoadBalancerName(ctx, clusterName, service)
	balancer, err := lb.findLoadBalancer(client, settings, name)
	if err != nil {
		return err
	}
	if balancer == nil {
		return nil
	}

	shared := settings.LoadBalancerID != ""

	existing, err := listListeners(client, balancer.ID)
	if err != nil {
		return err
	}
	for i := range existing {
		listener := &existing[i]
		if shared && !strings.HasPrefix(listener.Name, name+"_") {
			continue // not ours
		}
		if err := lb.deleteListener(ctx, client, balancer.ID, name, listener); err != nil {
			return err
		}
	}

	if shared {
		return nil
	}

	// No waitForActive here: the load balancer is gone after this call.
	if err := retryOnConflict(ctx, balancer.ID, func() error {
		return loadbalancers.Delete(client, balancer.ID).ExtractErr()
	}); err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to delete load balancer %s: %w", balancer.ID, err)
	}
	klog.V(2).Infof("Deleted load balancer %s (%s) for service %s/%s", name, balancer.ID, service.Namespace, service.Name)

	// OTC only unbinds EIPs on LB deletion; release auto-created ones so
	// they do not leak (user-provided eip-ids are never touched).
	if settings.EipBandwidth > 0 && !settings.EipKeep {
		if err := lb.releaseEips(ctx, balancer.PublicIps); err != nil {
			return err
		}
	}
	return nil
}

// releaseEips releases EIPs, retrying while unbinding is in progress.
func (lb *LoadBalancer) releaseEips(ctx context.Context, eipInfos []loadbalancers.PublicIpInfo) error {
	if len(eipInfos) == 0 {
		return nil
	}
	netClient, err := lb.newNetworkClient(ctx)
	if err != nil {
		return err
	}
	for _, eip := range eipInfos {
		if eip.PublicIpID == "" {
			continue
		}
		err := retryOnConflict(ctx, eip.PublicIpID, func() error {
			if err := eips.Delete(netClient, eip.PublicIpID).ExtractErr(); err != nil {
				// A still-bound EIP surfaces as 400/409; retry those too.
				if isConflict(err) || isBadRequest(err) {
					return golangsdk.ErrDefault409{}
				}
				return err
			}
			return nil
		})
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to release EIP %s (%s): %w", eip.PublicIpID, eip.PublicIpAddress, err)
		}
		klog.V(2).Infof("Released EIP %s (%s)", eip.PublicIpID, eip.PublicIpAddress)
	}
	return nil
}

// --- reconciliation ---

// reconcile drives listeners, pools, members and monitors to the desired state.
func (lb *LoadBalancer) reconcile(ctx context.Context, client *golangsdk.ServiceClient, balancer *loadbalancers.LoadBalancer, name string, settings *serviceSettings, service *v1.Service, nodes []*v1.Node) error {
	existing, err := listListeners(client, balancer.ID)
	if err != nil {
		return err
	}
	byKey := listenersByKey(existing)

	wanted := map[string]bool{}
	for _, port := range service.Spec.Ports {
		key := portKey(port)
		wanted[key] = true

		listener, ok := byKey[key]
		if !ok {
			created, err := lb.createListener(ctx, client, balancer.ID, name, settings, port)
			if err != nil {
				return err
			}
			listener = *created
			klog.V(2).Infof("Created listener %s (%s)", listener.Name, listener.ID)
		} else if settings.IdleTimeout > 0 {
			// The SDK does not expose the current value; apply unconditionally.
			err := mutate(ctx, client, balancer.ID, func() error {
				_, err := listeners.Update(client, listener.ID, listeners.UpdateOpts{
					KeepAliveTimeout: settings.IdleTimeout,
				}).Extract()
				return err
			})
			if err != nil {
				return fmt.Errorf("failed to update listener %s: %w", listener.ID, err)
			}
		}

		if err := lb.reconcileIPGroup(ctx, client, balancer.ID, &listener, name, service, port); err != nil {
			return err
		}

		pool, err := lb.ensurePool(ctx, client, balancer.ID, listener, name, settings, service, port)
		if err != nil {
			return err
		}

		if err := lb.reconcileMembers(ctx, client, balancer, pool.ID, port.NodePort, nodes); err != nil {
			return err
		}

		if err := lb.reconcileMonitor(ctx, client, balancer.ID, pool, name, settings, service, port); err != nil {
			return err
		}
	}

	// Remove listeners we own that no longer match a Service port.
	for i := range existing {
		listener := &existing[i]
		if !strings.HasPrefix(listener.Name, name+"_") {
			continue
		}
		if wanted[listenerKey(listener)] {
			continue
		}
		klog.V(2).Infof("Deleting stale listener %s (%s)", listener.Name, listener.ID)
		if err := lb.deleteListener(ctx, client, balancer.ID, name, listener); err != nil {
			return err
		}
	}
	return nil
}

func (lb *LoadBalancer) createLoadBalancer(client *golangsdk.ServiceClient, settings *serviceSettings, name string, service *v1.Service) (*loadbalancers.LoadBalancer, error) {
	if settings.SubnetID == "" {
		return nil, fmt.Errorf("cannot create load balancer for service %s/%s: subnet-id is not set (config [LoadBalancer] subnet-id or annotation %s)",
			service.Namespace, service.Name, AnnotationSubnetID)
	}
	if len(settings.AvailabilityZones) == 0 {
		return nil, fmt.Errorf("cannot create load balancer for service %s/%s: no availability zones set (config [LoadBalancer] availability-zone or annotation %s)",
			service.Namespace, service.Name, AnnotationAvailabilityZones)
	}

	createOpts := loadbalancers.CreateOpts{
		Name:                 name,
		Description:          fmt.Sprintf("Kubernetes service %s/%s", service.Namespace, service.Name),
		VipSubnetCidrID:      settings.SubnetID,
		VpcID:                settings.VpcID,
		AvailabilityZoneList: settings.AvailabilityZones,
		L4Flavor:             settings.L4FlavorID,
		VipAddress:           service.Spec.LoadBalancerIP, //nolint:staticcheck // deprecated field still honored
		PublicIpIDs:          settings.EipIDs,
	}
	if settings.EipBandwidth > 0 {
		createOpts.PublicIp = &loadbalancers.PublicIp{
			NetworkType: settings.EipType,
			Description: fmt.Sprintf("Kubernetes service %s/%s", service.Namespace, service.Name),
			Bandwidth: loadbalancers.Bandwidth{
				Name:       cutString(name, 64),
				Size:       settings.EipBandwidth,
				ChargeMode: "traffic",
				ShareType:  "PER",
			},
		}
	}

	balancer, err := loadbalancers.Create(client, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}
	return balancer, nil
}

func (lb *LoadBalancer) createListener(ctx context.Context, client *golangsdk.ServiceClient, lbID, name string, settings *serviceSettings, port v1.ServicePort) (*listeners.Listener, error) {
	var listener *listeners.Listener
	err := mutate(ctx, client, lbID, func() error {
		var err error
		listener, err = listeners.Create(client, listeners.CreateOpts{
			LoadbalancerID:   lbID,
			Name:             cutString(fmt.Sprintf("%s_%s_%d", name, port.Protocol, port.Port), maxResourceNameLength),
			Protocol:         listeners.Protocol(port.Protocol),
			ProtocolPort:     int(port.Port),
			KeepAliveTimeout: settings.IdleTimeout,
		}).Extract()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create listener for port %d: %w", port.Port, err)
	}
	return listener, nil
}

// desiredPersistence maps Service session affinity to pool persistence.
func desiredPersistence(service *v1.Service) *pools.SessionPersistence {
	if service.Spec.SessionAffinity != v1.ServiceAffinityClientIP {
		return nil
	}
	persistence := &pools.SessionPersistence{Type: "SOURCE_IP"}
	if cfg := service.Spec.SessionAffinityConfig; cfg != nil && cfg.ClientIP != nil && cfg.ClientIP.TimeoutSeconds != nil {
		// ELB wants minutes (1-60), Kubernetes gives seconds; round up and clamp.
		minutes := int((*cfg.ClientIP.TimeoutSeconds + 59) / 60)
		if minutes < 1 {
			minutes = 1
		}
		if minutes > 60 {
			minutes = 60
		}
		persistence.PersistenceTimeout = minutes
	}
	return persistence
}

func persistenceEqual(a, b *pools.SessionPersistence) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Type == b.Type && a.PersistenceTimeout == b.PersistenceTimeout
}

// ensurePool creates or reconciles the listener's default pool.
func (lb *LoadBalancer) ensurePool(ctx context.Context, client *golangsdk.ServiceClient, lbID string, listener listeners.Listener, name string, settings *serviceSettings, service *v1.Service, port v1.ServicePort) (*pools.Pool, error) {
	persistence := desiredPersistence(service)

	if listener.DefaultPoolID != "" {
		pool, err := pools.Get(client, listener.DefaultPoolID).Extract()
		if err != nil {
			return nil, fmt.Errorf("failed to get pool %s: %w", listener.DefaultPoolID, err)
		}
		if persistence == nil && pool.Persistence != nil {
			// The SDK cannot send an explicit null to clear persistence.
			klog.Warningf("Cannot disable session persistence on existing pool %s; recreate the Service to remove it", pool.ID)
		}
		if pool.LBMethod != settings.LBAlgorithm || (persistence != nil && !persistenceEqual(pool.Persistence, persistence)) {
			updateOpts := pools.UpdateOpts{
				LBMethod:    settings.LBAlgorithm,
				Persistence: persistence,
			}
			err := mutate(ctx, client, lbID, func() error {
				var err error
				pool, err = pools.Update(client, pool.ID, updateOpts).Extract()
				return err
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update pool %s: %w", pool.ID, err)
			}
			klog.V(2).Infof("Updated pool %s (%s)", pool.Name, pool.ID)
		}
		return pool, nil
	}

	createOpts := pools.CreateOpts{
		ListenerID:  listener.ID,
		Name:        cutString(fmt.Sprintf("%s_%s_%d", name, port.Protocol, port.Port), maxResourceNameLength),
		Protocol:    string(port.Protocol),
		LBMethod:    settings.LBAlgorithm,
		Description: fmt.Sprintf("Kubernetes service %s/%s", service.Namespace, service.Name),
		Persistence: persistence,
	}

	var pool *pools.Pool
	err := mutate(ctx, client, lbID, func() error {
		var err error
		pool, err = pools.Create(client, createOpts).Extract()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pool for listener %s: %w", listener.ID, err)
	}
	klog.V(2).Infof("Created pool %s (%s)", pool.Name, pool.ID)
	return pool, nil
}

// reconcileMembers syncs pool members with the node set on the NodePort.
func (lb *LoadBalancer) reconcileMembers(ctx context.Context, client *golangsdk.ServiceClient, balancer *loadbalancers.LoadBalancer, poolID string, nodePort int32, nodes []*v1.Node) error {
	if nodePort == 0 {
		return fmt.Errorf("service has no NodePort allocated; cannot add members to pool %s", poolID)
	}

	existing, err := listMembers(client, poolID)
	if err != nil {
		return err
	}

	wanted := map[string]*v1.Node{}
	for _, node := range nodes {
		addr, err := nodeAddress(node)
		if err != nil {
			klog.Warningf("Skipping node %s: %v", node.Name, err)
			continue
		}
		wanted[fmt.Sprintf("%s:%d", addr, nodePort)] = node
	}
	if len(wanted) == 0 {
		return fmt.Errorf("no usable node addresses for pool %s", poolID)
	}

	have := map[string]string{} // addr:port -> member ID
	for _, m := range existing {
		have[fmt.Sprintf("%s:%d", m.Address, m.ProtocolPort)] = m.ID
	}

	for key, node := range wanted {
		if _, ok := have[key]; ok {
			continue
		}
		addr, _ := nodeAddress(node)
		err := mutate(ctx, client, balancer.ID, func() error {
			_, err := members.Create(client, poolID, members.CreateOpts{
				Address:      addr,
				ProtocolPort: int(nodePort),
				Name:         cutString(node.Name, maxResourceNameLength),
				SubnetID:     balancer.VipSubnetCidrID,
			}).Extract()
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to add member %s to pool %s: %w", key, poolID, err)
		}
		klog.V(2).Infof("Added member %s to pool %s", key, poolID)
	}

	for key, id := range have {
		if _, ok := wanted[key]; ok {
			continue
		}
		err := mutate(ctx, client, balancer.ID, func() error {
			return members.Delete(client, poolID, id).ExtractErr()
		})
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to remove member %s from pool %s: %w", key, poolID, err)
		}
		klog.V(2).Infof("Removed member %s from pool %s", key, poolID)
	}
	return nil
}

// reconcileMonitor creates, updates or deletes the pool's health monitor.
func (lb *LoadBalancer) reconcileMonitor(ctx context.Context, client *golangsdk.ServiceClient, lbID string, pool *pools.Pool, name string, settings *serviceSettings, service *v1.Service, port v1.ServicePort) error {
	monitorType := monitors.TypeTCP
	monitorPort := 0
	urlPath := ""
	httpMethod := ""
	switch {
	case port.Protocol == v1.ProtocolUDP:
		// The SDK has no constant for UDP_CONNECT yet.
		monitorType = monitors.Type("UDP_CONNECT")
	case service.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyLocal && service.Spec.HealthCheckNodePort > 0:
		// Local policy: probe kube-proxy healthz so only nodes with local
		// endpoints receive traffic.
		monitorType = monitors.TypeHTTP
		monitorPort = int(service.Spec.HealthCheckNodePort)
		urlPath = "/healthz"
		httpMethod = "GET"
	case settings.HealthCheckProtocol == "HTTP" || settings.HealthCheckProtocol == "HTTPS":
		monitorType = monitors.Type(settings.HealthCheckProtocol)
		urlPath = settings.HealthCheckURLPath
		if urlPath == "" {
			urlPath = "/"
		}
		httpMethod = settings.HealthCheckHTTPMethod
		if httpMethod == "" {
			httpMethod = "GET"
		}
	}

	if !settings.HealthCheckEnabled {
		if pool.MonitorID == "" {
			return nil
		}
		err := mutate(ctx, client, lbID, func() error {
			return monitors.Delete(client, pool.MonitorID).ExtractErr()
		})
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to delete monitor %s: %w", pool.MonitorID, err)
		}
		klog.V(2).Infof("Deleted health monitor %s of pool %s", pool.MonitorID, pool.ID)
		return nil
	}

	createMonitor := func() error {
		err := mutate(ctx, client, lbID, func() error {
			_, err := monitors.Create(client, monitors.CreateOpts{
				PoolID:      pool.ID,
				Name:        cutString(fmt.Sprintf("%s_%s_%d", name, port.Protocol, port.Port), maxResourceNameLength),
				Type:        monitorType,
				Delay:       settings.HealthCheckDelay,
				Timeout:     settings.HealthCheckTimeout,
				MaxRetries:  settings.HealthCheckMaxRetries,
				URLPath:     urlPath,
				HTTPMethod:  httpMethod,
				MonitorPort: monitorPort,
			}).Extract()
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to create monitor for pool %s: %w", pool.ID, err)
		}
		klog.V(2).Infof("Created health monitor for pool %s", pool.ID)
		return nil
	}

	if pool.MonitorID == "" {
		return createMonitor()
	}

	monitor, err := monitors.Get(client, pool.MonitorID).Extract()
	if err != nil {
		return fmt.Errorf("failed to get monitor %s: %w", pool.MonitorID, err)
	}
	if monitor.Type != monitorType || monitor.MonitorPort != monitorPort {
		// Port cannot be reset via update; recreate on type/port changes.
		err := mutate(ctx, client, lbID, func() error {
			return monitors.Delete(client, pool.MonitorID).ExtractErr()
		})
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to delete monitor %s for recreation: %w", pool.MonitorID, err)
		}
		return createMonitor()
	}
	if monitor.Delay == settings.HealthCheckDelay &&
		monitor.Timeout == settings.HealthCheckTimeout &&
		monitor.MaxRetries == settings.HealthCheckMaxRetries &&
		monitor.URLPath == urlPath &&
		monitor.HTTPMethod == httpMethod {
		return nil
	}
	err = mutate(ctx, client, lbID, func() error {
		_, err := monitors.Update(client, pool.MonitorID, monitors.UpdateOpts{
			Delay:      settings.HealthCheckDelay,
			Timeout:    settings.HealthCheckTimeout,
			MaxRetries: settings.HealthCheckMaxRetries,
			URLPath:    urlPath,
			HTTPMethod: httpMethod,
		}).Extract()
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to update monitor %s: %w", pool.MonitorID, err)
	}
	klog.V(2).Infof("Updated health monitor %s of pool %s", pool.MonitorID, pool.ID)
	return nil
}

// deleteListener tears down a listener with its pool, members, monitor and
// owned IP group.
func (lb *LoadBalancer) deleteListener(ctx context.Context, client *golangsdk.ServiceClient, lbID, name string, listener *listeners.Listener) error {
	pool, err := findPoolForListener(client, listener.ID)
	if err != nil {
		return err
	}
	if pool != nil {
		if pool.MonitorID != "" {
			err := mutate(ctx, client, lbID, func() error {
				return monitors.Delete(client, pool.MonitorID).ExtractErr()
			})
			if err != nil && !isNotFound(err) {
				return fmt.Errorf("failed to delete monitor %s: %w", pool.MonitorID, err)
			}
		}
		existing, err := listMembers(client, pool.ID)
		if err != nil {
			return err
		}
		for _, m := range existing {
			err := mutate(ctx, client, lbID, func() error {
				return members.Delete(client, pool.ID, m.ID).ExtractErr()
			})
			if err != nil && !isNotFound(err) {
				return fmt.Errorf("failed to delete member %s: %w", m.ID, err)
			}
		}
		err = mutate(ctx, client, lbID, func() error {
			return pools.Delete(client, pool.ID).ExtractErr()
		})
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("failed to delete pool %s: %w", pool.ID, err)
		}
	}

	err = mutate(ctx, client, lbID, func() error {
		return listeners.Delete(client, listener.ID).ExtractErr()
	})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to delete listener %s: %w", listener.ID, err)
	}
	// The group can only be deleted once the listener no longer binds it.
	return lb.deleteIPGroup(ctx, client, lbID, listener.IpGroup.IpGroupID, name)
}

// --- lookup helpers ---

// findLoadBalancer looks up the LB by id annotation or name; nil if absent.
func (lb *LoadBalancer) findLoadBalancer(client *golangsdk.ServiceClient, settings *serviceSettings, name string) (*loadbalancers.LoadBalancer, error) {
	if settings.LoadBalancerID != "" {
		balancer, err := loadbalancers.Get(client, settings.LoadBalancerID).Extract()
		if err != nil {
			if isNotFound(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get load balancer %s: %w", settings.LoadBalancerID, err)
		}
		return balancer, nil
	}

	pages, err := loadbalancers.List(client, loadbalancers.ListOpts{Name: []string{name}}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list load balancers: %w", err)
	}
	items, err := loadbalancers.ExtractLoadbalancers(pages)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Name == name {
			return &items[i], nil
		}
	}
	return nil, nil
}

func findPoolForListener(client *golangsdk.ServiceClient, listenerID string) (*pools.Pool, error) {
	listener, err := listeners.Get(client, listenerID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get listener %s: %w", listenerID, err)
	}
	if listener.DefaultPoolID == "" {
		return nil, nil
	}
	pool, err := pools.Get(client, listener.DefaultPoolID).Extract()
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get pool %s: %w", listener.DefaultPoolID, err)
	}
	return pool, nil
}

func listListeners(client *golangsdk.ServiceClient, lbID string) ([]listeners.Listener, error) {
	pages, err := listeners.List(client, listeners.ListOpts{LoadBalancerID: []string{lbID}}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list listeners of load balancer %s: %w", lbID, err)
	}
	return listeners.ExtractListeners(pages)
}

func listMembers(client *golangsdk.ServiceClient, poolID string) ([]members.Member, error) {
	pages, err := members.List(client, poolID, members.ListOpts{}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to list members of pool %s: %w", poolID, err)
	}
	return members.ExtractMembers(pages)
}

// --- small helpers ---

func portKey(port v1.ServicePort) string {
	return fmt.Sprintf("%s/%d", port.Protocol, port.Port)
}

func listenerKey(listener *listeners.Listener) string {
	return fmt.Sprintf("%s/%d", listener.Protocol, listener.ProtocolPort)
}

func listenersByKey(items []listeners.Listener) map[string]listeners.Listener {
	m := make(map[string]listeners.Listener, len(items))
	for _, l := range items {
		m[listenerKey(&l)] = l
	}
	return m
}

func validatePorts(service *v1.Service) error {
	for _, port := range service.Spec.Ports {
		switch port.Protocol {
		case v1.ProtocolTCP, v1.ProtocolUDP:
		default:
			return fmt.Errorf("protocol %s of port %d is not supported by ELB (only TCP and UDP)", port.Protocol, port.Port)
		}
	}
	return nil
}

// nodeAddress returns the member address, preferring the internal IP.
func nodeAddress(node *v1.Node) (string, error) {
	var external string
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case v1.NodeInternalIP:
			if addr.Address != "" {
				return addr.Address, nil
			}
		case v1.NodeExternalIP:
			if external == "" {
				external = addr.Address
			}
		}
	}
	if external != "" {
		return external, nil
	}
	return "", fmt.Errorf("node %s has no internal or external IP address", node.Name)
}

// lbStatus builds the Service status, preferring EIPs over the private VIP.
func lbStatus(balancer *loadbalancers.LoadBalancer) *v1.LoadBalancerStatus {
	status := &v1.LoadBalancerStatus{}
	for _, eip := range balancer.PublicIps {
		if eip.PublicIpAddress != "" {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: eip.PublicIpAddress})
		}
	}
	if len(status.Ingress) == 0 && balancer.VipAddress != "" {
		status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: balancer.VipAddress})
	}
	return status
}

func cutString(s string, length int) string {
	if len(s) > length {
		return s[:length]
	}
	return s
}

// waitForActive polls until the LB provisioning status is ACTIVE.
func waitForActive(ctx context.Context, client *golangsdk.ServiceClient, lbID string) error {
	return wait.PollUntilContextTimeout(ctx, activePollInterval, activePollTimeout, true, func(_ context.Context) (bool, error) {
		balancer, err := loadbalancers.Get(client, lbID).Extract()
		if err != nil {
			return false, err
		}
		switch balancer.ProvisioningStatus {
		case statusActive:
			return true, nil
		case statusError:
			return false, fmt.Errorf("load balancer %s is in ERROR state", lbID)
		default:
			return false, nil
		}
	})
}

// retryOnConflict retries a mutating call on 409 with backoff.
func retryOnConflict(ctx context.Context, lbID string, fn func() error) error {
	backoff := wait.Backoff{Duration: 2 * time.Second, Factor: 1.5, Steps: 6}
	return wait.ExponentialBackoffWithContext(ctx, backoff, func(_ context.Context) (bool, error) {
		if err := fn(); err != nil {
			if isConflict(err) {
				klog.V(4).Infof("Load balancer %s busy, retrying: %v", lbID, err)
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

// mutate is retryOnConflict plus a wait for the LB to return to ACTIVE.
func mutate(ctx context.Context, client *golangsdk.ServiceClient, lbID string, fn func() error) error {
	if err := retryOnConflict(ctx, lbID, fn); err != nil {
		return err
	}
	return waitForActive(ctx, client, lbID)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var e404 golangsdk.ErrDefault404
	if errors.As(err, &e404) {
		return true
	}
	var eNF golangsdk.ErrResourceNotFound
	return errors.As(err, &eNF)
}

func isConflict(err error) bool {
	var e409 golangsdk.ErrDefault409
	return errors.As(err, &e409)
}

func isBadRequest(err error) bool {
	var e400 golangsdk.ErrDefault400
	return errors.As(err, &e400)
}

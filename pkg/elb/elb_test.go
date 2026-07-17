package elb

import (
	"context"
	"fmt"
	"testing"

	golangsdk "github.com/opentelekomcloud/gophertelekomcloud"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

var _ cloudprovider.LoadBalancer = &LoadBalancer{}

func testLoadBalancer(f *fakeELB) *LoadBalancer {
	opts := config.DefaultLoadBalancerOpts()
	opts.SubnetID = "subnet-1234"
	opts.VpcID = "vpc-1234"
	opts.AvailabilityZones = []string{"eu-de-01"}
	return &LoadBalancer{
		opts: opts,
		newClient: func(_ context.Context) (*golangsdk.ServiceClient, error) {
			return f.client(), nil
		},
		newNetworkClient: func(_ context.Context) (*golangsdk.ServiceClient, error) {
			return f.networkClient(), nil
		},
	}
}

func testService(ports ...v1.ServicePort) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "default",
			UID:       "1234-5678",
		},
		Spec: v1.ServiceSpec{
			Type:  v1.ServiceTypeLoadBalancer,
			Ports: ports,
		},
	}
}

func testNode(name, ip string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: ip},
			},
		},
	}
}

func TestGetLoadBalancerName(t *testing.T) {
	lb := &LoadBalancer{}
	svc := testService()
	name := lb.GetLoadBalancerName(context.Background(), "mycluster", svc)
	want := "kube_service_mycluster_default_web"
	if name != want {
		t.Errorf("expected %q, got %q", want, name)
	}
}

// TestLoadBalancerLifecycle: create, node update, port removal, deletion.
func TestLoadBalancerLifecycle(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()

	svc := testService(
		v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080},
		v1.ServicePort{Protocol: v1.ProtocolUDP, Port: 53, NodePort: 30053},
	)
	nodes := []*v1.Node{
		testNode("node-1", "10.0.0.1"),
		testNode("node-2", "10.0.0.2"),
	}

	// Before creation nothing exists.
	if _, exists, err := lb.GetLoadBalancer(ctx, "cluster", svc); err != nil || exists {
		t.Fatalf("expected no LB before creation, exists=%v err=%v", exists, err)
	}

	// --- Create ---
	status, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes)
	if err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}
	if len(status.Ingress) != 1 || status.Ingress[0].IP == "" {
		t.Fatalf("expected one ingress IP, got %+v", status.Ingress)
	}

	f.mu.Lock()
	if len(f.loadBalancers) != 1 {
		t.Errorf("expected 1 load balancer, got %d", len(f.loadBalancers))
	}
	if len(f.listeners) != 2 {
		t.Errorf("expected 2 listeners, got %d", len(f.listeners))
	}
	if len(f.pools) != 2 {
		t.Errorf("expected 2 pools, got %d", len(f.pools))
	}
	if len(f.monitors) != 2 {
		t.Errorf("expected 2 health monitors, got %d", len(f.monitors))
	}
	total := 0
	for _, pm := range f.members {
		total += len(pm)
	}
	if total != 4 {
		t.Errorf("expected 4 members (2 nodes x 2 pools), got %d", total)
	}
	f.mu.Unlock()

	// Idempotency: a second Ensure must not duplicate anything.
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("second EnsureLoadBalancer failed: %v", err)
	}
	f.mu.Lock()
	if len(f.listeners) != 2 || len(f.pools) != 2 || len(f.monitors) != 2 {
		t.Errorf("resources duplicated on repeated ensure: %d listeners, %d pools, %d monitors",
			len(f.listeners), len(f.pools), len(f.monitors))
	}
	f.mu.Unlock()

	// GetLoadBalancer now reports existence.
	if _, exists, err := lb.GetLoadBalancer(ctx, "cluster", svc); err != nil || !exists {
		t.Fatalf("expected LB to exist, exists=%v err=%v", exists, err)
	}

	// --- Node set change ---
	if err := lb.UpdateLoadBalancer(ctx, "cluster", svc, nodes[:1]); err != nil {
		t.Fatalf("UpdateLoadBalancer failed: %v", err)
	}
	f.mu.Lock()
	total = 0
	for _, pm := range f.members {
		total += len(pm)
		for _, m := range pm {
			if m["address"] != "10.0.0.1" {
				t.Errorf("unexpected member address %v after node removal", m["address"])
			}
		}
	}
	if total != 2 {
		t.Errorf("expected 2 members after node removal, got %d", total)
	}
	f.mu.Unlock()

	// --- Port removal (stale listener cleanup) ---
	svcOnePort := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svcOnePort, nodes[:1]); err != nil {
		t.Fatalf("EnsureLoadBalancer with one port failed: %v", err)
	}
	f.mu.Lock()
	if len(f.listeners) != 1 || len(f.pools) != 1 || len(f.monitors) != 1 {
		t.Errorf("expected stale resources removed: %d listeners, %d pools, %d monitors",
			len(f.listeners), len(f.pools), len(f.monitors))
	}
	f.mu.Unlock()

	// --- Delete ---
	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svcOnePort); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	f.mu.Lock()
	if len(f.loadBalancers) != 0 || len(f.listeners) != 0 || len(f.pools) != 0 || len(f.monitors) != 0 {
		t.Errorf("expected everything deleted: %d LBs, %d listeners, %d pools, %d monitors",
			len(f.loadBalancers), len(f.listeners), len(f.pools), len(f.monitors))
	}
	f.mu.Unlock()

	// Deleting again must be a no-op.
	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svcOnePort); err != nil {
		t.Fatalf("repeated EnsureLoadBalancerDeleted failed: %v", err)
	}
}

// TestSharedLoadBalancer: an LB from the id annotation is reused, not deleted.
func TestSharedLoadBalancer(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()

	// Pre-create a "user-managed" load balancer.
	f.mu.Lock()
	f.loadBalancers["lb-shared"] = map[string]any{
		"id":                  "lb-shared",
		"name":                "user-lb",
		"provisioning_status": "ACTIVE",
		"vip_address":         "192.168.0.42",
		"vip_subnet_cidr_id":  "subnet-1234",
	}
	f.mu.Unlock()

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 443, NodePort: 30443})
	svc.Annotations = map[string]string{AnnotationID: "lb-shared"}

	status, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, []*v1.Node{testNode("node-1", "10.0.0.1")})
	if err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}
	if status.Ingress[0].IP != "192.168.0.42" {
		t.Errorf("expected shared LB VIP, got %+v", status.Ingress)
	}

	f.mu.Lock()
	if len(f.loadBalancers) != 1 {
		t.Errorf("expected the shared LB to be reused, got %d LBs", len(f.loadBalancers))
	}
	f.mu.Unlock()

	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svc); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	f.mu.Lock()
	if len(f.loadBalancers) != 1 {
		t.Error("shared load balancer must not be deleted")
	}
	if len(f.listeners) != 0 || len(f.pools) != 0 {
		t.Errorf("expected own listeners/pools removed from shared LB, got %d/%d", len(f.listeners), len(f.pools))
	}
	f.mu.Unlock()
}

// TestEipLifecycle: eip-bandwidth EIP is created, published and released.
func TestEipLifecycle(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Annotations = map[string]string{AnnotationEipBandwidth: "100"}

	status, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes)
	if err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}

	f.mu.Lock()
	if len(f.eips) != 1 {
		t.Fatalf("expected 1 EIP created, got %d", len(f.eips))
	}
	var eipAddr string
	for _, e := range f.eips {
		eipAddr = e["public_ip_address"].(string)
	}
	f.mu.Unlock()

	if len(status.Ingress) != 1 || status.Ingress[0].IP != eipAddr {
		t.Errorf("expected EIP %s in status, got %+v", eipAddr, status.Ingress)
	}

	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svc); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	f.mu.Lock()
	if len(f.eips) != 0 {
		t.Errorf("expected EIP released on deletion, %d left", len(f.eips))
	}
	if len(f.loadBalancers) != 0 {
		t.Errorf("expected load balancer deleted, %d left", len(f.loadBalancers))
	}
	f.mu.Unlock()
}

// TestHTTPHealthCheck: HTTP probe annotations apply and update in place.
func TestHTTPHealthCheck(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Annotations = map[string]string{
		AnnotationHealthCheckProtocol:   "HTTP",
		AnnotationHealthCheckURLPath:    "/healthz",
		AnnotationHealthCheckHTTPMethod: "HEAD",
	}

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}

	checkMonitor := func(wantType, wantPath, wantMethod string) {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.monitors) != 1 {
			t.Fatalf("expected 1 monitor, got %d", len(f.monitors))
		}
		for _, m := range f.monitors {
			if m["type"] != wantType || m["url_path"] != wantPath || m["http_method"] != wantMethod {
				t.Errorf("monitor = type:%v path:%v method:%v, want %s %s %s",
					m["type"], m["url_path"], m["http_method"], wantType, wantPath, wantMethod)
			}
		}
	}
	checkMonitor("HTTP", "/healthz", "HEAD")

	// Change probe settings: the existing monitor must be updated in place.
	svc.Annotations[AnnotationHealthCheckURLPath] = "/livez"
	svc.Annotations[AnnotationHealthCheckHTTPMethod] = "GET"
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer after annotation change failed: %v", err)
	}
	checkMonitor("HTTP", "/livez", "GET")
}

// TestExternalTrafficPolicyLocal: Local probes healthz node port over HTTP;
// reverting to Cluster recreates a TCP monitor.
func TestExternalTrafficPolicyLocal(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyLocal
	svc.Spec.HealthCheckNodePort = 32001

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}

	checkMonitor := func(wantType string, wantPort any, wantPath string) {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.monitors) != 1 {
			t.Fatalf("expected 1 monitor, got %d", len(f.monitors))
		}
		for _, m := range f.monitors {
			gotPath, _ := m["url_path"].(string) // nil (omitted) reads as ""
			if m["type"] != wantType || gotPath != wantPath {
				t.Errorf("monitor = type:%v path:%q, want %s %q", m["type"], gotPath, wantType, wantPath)
			}
			if fmt.Sprintf("%v", m["monitor_port"]) != fmt.Sprintf("%v", wantPort) {
				t.Errorf("monitor_port = %v, want %v", m["monitor_port"], wantPort)
			}
		}
	}
	checkMonitor("HTTP", 32001, "/healthz")

	// Switching back to Cluster must recreate a TCP monitor on member port.
	svc.Spec.ExternalTrafficPolicy = v1.ServiceExternalTrafficPolicyCluster
	svc.Spec.HealthCheckNodePort = 0
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer after policy change failed: %v", err)
	}
	checkMonitor("TCP", "<nil>", "")
}

// TestSessionAffinity: SOURCE_IP persistence with timeout, updated in place.
func TestSessionAffinity(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	timeout := int32(600) // 10 minutes
	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Spec.SessionAffinity = v1.ServiceAffinityClientIP
	svc.Spec.SessionAffinityConfig = &v1.SessionAffinityConfig{
		ClientIP: &v1.ClientIPConfig{TimeoutSeconds: &timeout},
	}

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}

	checkPersistence := func(wantTimeout string) {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, p := range f.pools {
			sp, ok := p["session_persistence"].(map[string]any)
			if !ok {
				t.Fatalf("expected session_persistence on pool, got %v", p["session_persistence"])
			}
			if sp["type"] != "SOURCE_IP" || fmt.Sprintf("%v", sp["persistence_timeout"]) != wantTimeout {
				t.Errorf("persistence = %v, want SOURCE_IP timeout %s", sp, wantTimeout)
			}
		}
	}
	checkPersistence("10")

	// Timeout change must update the pool in place.
	newTimeout := int32(1800) // 30 minutes
	svc.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds = &newTimeout
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer after timeout change failed: %v", err)
	}
	checkPersistence("30")
}

func TestDesiredPersistenceClamping(t *testing.T) {
	svc := testService()
	svc.Spec.SessionAffinity = v1.ServiceAffinityClientIP

	if p := desiredPersistence(svc); p == nil || p.Type != "SOURCE_IP" || p.PersistenceTimeout != 0 {
		t.Errorf("expected SOURCE_IP without explicit timeout, got %+v", p)
	}

	short := int32(30)
	svc.Spec.SessionAffinityConfig = &v1.SessionAffinityConfig{ClientIP: &v1.ClientIPConfig{TimeoutSeconds: &short}}
	if p := desiredPersistence(svc); p.PersistenceTimeout != 1 {
		t.Errorf("expected 30s to clamp to 1 minute, got %d", p.PersistenceTimeout)
	}

	long := int32(7200)
	svc.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds = &long
	if p := desiredPersistence(svc); p.PersistenceTimeout != 60 {
		t.Errorf("expected 7200s to clamp to 60 minutes, got %d", p.PersistenceTimeout)
	}

	svc.Spec.SessionAffinity = v1.ServiceAffinityNone
	if p := desiredPersistence(svc); p != nil {
		t.Errorf("expected nil persistence for affinity None, got %+v", p)
	}
}

// TestIdleTimeout: idle-timeout annotation lands on the listener.
func TestIdleTimeout(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Annotations = map[string]string{AnnotationIdleTimeout: "120"}

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}
	f.mu.Lock()
	for _, l := range f.listeners {
		if fmt.Sprintf("%v", l["keepalive_timeout"]) != "120" {
			t.Errorf("keepalive_timeout = %v, want 120", l["keepalive_timeout"])
		}
	}
	f.mu.Unlock()
}

// TestEipKeep: eip-keep preserves an auto-created EIP.
func TestEipKeep(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Annotations = map[string]string{
		AnnotationEipBandwidth: "50",
		AnnotationEipKeep:      "true",
	}

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}
	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svc); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	f.mu.Lock()
	if len(f.eips) != 1 {
		t.Errorf("expected EIP kept with eip-keep=true, got %d EIPs", len(f.eips))
	}
	if len(f.loadBalancers) != 0 {
		t.Errorf("expected load balancer deleted, %d left", len(f.loadBalancers))
	}
	f.mu.Unlock()
}

// TestSourceRanges: loadBalancerSourceRanges maps to a listener ipgroup.
func TestSourceRanges(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	nodes := []*v1.Node{testNode("node-1", "10.0.0.1")}

	svc := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	svc.Spec.LoadBalancerSourceRanges = []string{"10.0.0.0/8", "192.168.1.0/24"}

	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}

	ipCount := func() int {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.ipGroups) != 1 {
			t.Fatalf("expected 1 ipgroup, got %d", len(f.ipGroups))
		}
		for _, g := range f.ipGroups {
			return len(g["ip_list"].([]any))
		}
		return 0
	}
	if n := ipCount(); n != 2 {
		t.Errorf("expected 2 CIDRs in ipgroup, got %d", n)
	}

	// Listener must have the whitelist attached and enabled.
	f.mu.Lock()
	for _, l := range f.listeners {
		ig, ok := l["ipgroup"].(map[string]any)
		if !ok || ig["enable_ipgroup"] != true || ig["type"] != "white" {
			t.Errorf("expected enabled white ipgroup on listener, got %v", l["ipgroup"])
		}
	}
	f.mu.Unlock()

	// Range change updates the group in place.
	svc.Spec.LoadBalancerSourceRanges = []string{"172.16.0.0/12"}
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer after range change failed: %v", err)
	}
	if n := ipCount(); n != 1 {
		t.Errorf("expected 1 CIDR after update, got %d", n)
	}

	// Clearing the ranges disables the whitelist.
	svc.Spec.LoadBalancerSourceRanges = nil
	if _, err := lb.EnsureLoadBalancer(ctx, "cluster", svc, nodes); err != nil {
		t.Fatalf("EnsureLoadBalancer after clearing ranges failed: %v", err)
	}
	f.mu.Lock()
	for _, l := range f.listeners {
		ig, ok := l["ipgroup"].(map[string]any)
		if !ok || ig["enable_ipgroup"] != false {
			t.Errorf("expected disabled ipgroup after clearing ranges, got %v", l["ipgroup"])
		}
	}
	f.mu.Unlock()

	// Deleting the service removes the group.
	if err := lb.EnsureLoadBalancerDeleted(ctx, "cluster", svc); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	f.mu.Lock()
	if len(f.ipGroups) != 0 {
		t.Errorf("expected ipgroup deleted with the service, %d left", len(f.ipGroups))
	}
	f.mu.Unlock()
}

func TestEnsureLoadBalancerValidation(t *testing.T) {
	f := newFakeELB()
	defer f.close()
	lb := testLoadBalancer(f)
	ctx := context.Background()
	node := testNode("node-1", "10.0.0.1")

	// No ports.
	if _, err := lb.EnsureLoadBalancer(ctx, "c", testService(), []*v1.Node{node}); err == nil {
		t.Error("expected error for service without ports")
	}

	// Unsupported protocol.
	sctp := testService(v1.ServicePort{Protocol: v1.ProtocolSCTP, Port: 80, NodePort: 30080})
	if _, err := lb.EnsureLoadBalancer(ctx, "c", sctp, []*v1.Node{node}); err == nil {
		t.Error("expected error for SCTP port")
	}

	// No nodes.
	tcp := testService(v1.ServicePort{Protocol: v1.ProtocolTCP, Port: 80, NodePort: 30080})
	if _, err := lb.EnsureLoadBalancer(ctx, "c", tcp, nil); err == nil {
		t.Error("expected error for empty node list")
	}

	// Missing subnet configuration.
	lbNoSubnet := testLoadBalancer(f)
	lbNoSubnet.opts.SubnetID = ""
	if _, err := lbNoSubnet.EnsureLoadBalancer(ctx, "c", tcp, []*v1.Node{node}); err == nil {
		t.Error("expected error when subnet-id is not configured")
	}
}

//go:build smoke

// Package smoke exercises the LB implementation against the real OTC API
// without a Kubernetes cluster; see README.
package smoke

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/elb"
	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/identity"
)

const clusterName = "smoke"

// smokeEnv holds the resolved test configuration.
type smokeEnv struct {
	lb     *elb.LoadBalancer
	nodeIP string
}

// setup resolves credentials and SMOKE_* settings, skipping when missing.
func setup(t *testing.T) *smokeEnv {
	t.Helper()

	authOpts, err := config.AuthOptsFromCloudsYAML(os.Getenv("OS_CLOUD"))
	if err != nil {
		t.Skipf("no usable cloud credentials (clouds.yaml or OS_* env): %v", err)
	}

	subnetID := os.Getenv("SMOKE_SUBNET_ID")
	azs := os.Getenv("SMOKE_AVAILABILITY_ZONES")
	nodeIP := os.Getenv("SMOKE_NODE_IP")
	if subnetID == "" || azs == "" || nodeIP == "" {
		t.Skip("SMOKE_SUBNET_ID, SMOKE_AVAILABILITY_ZONES and SMOKE_NODE_IP are required " +
			"(subnet for the LB VIP, comma-separated AZs, and an IP inside the subnet to register as backend)")
	}

	idProvider, err := identity.NewIdentityProvider(*authOpts)
	if err != nil {
		t.Fatalf("failed to create identity provider: %v", err)
	}

	lbOpts := config.DefaultLoadBalancerOpts()
	lbOpts.SubnetID = subnetID
	lbOpts.VpcID = os.Getenv("SMOKE_VPC_ID")
	for _, az := range strings.Split(azs, ",") {
		if az = strings.TrimSpace(az); az != "" {
			lbOpts.AvailabilityZones = append(lbOpts.AvailabilityZones, az)
		}
	}

	return &smokeEnv{
		lb:     elb.NewLoadBalancer(func() identity.IdentityProvider { return idProvider }, lbOpts),
		nodeIP: nodeIP,
	}
}

func smokeService(name string) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "smoke-test",
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeLoadBalancer,
			Ports: []v1.ServicePort{{
				Name:     "http",
				Protocol: v1.ProtocolTCP,
				Port:     80,
				NodePort: 30080,
			}},
		},
	}
	if bw := os.Getenv("SMOKE_EIP_BANDWIDTH"); bw != "" {
		svc.Annotations = map[string]string{elb.AnnotationEipBandwidth: bw}
	}
	return svc
}

func smokeNode(ip string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "smoke-node"},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{{Type: v1.NodeInternalIP, Address: ip}},
		},
	}
}

// TestCloudSmoke: create, verify, idempotent re-ensure, update, delete.
func TestCloudSmoke(t *testing.T) {
	env := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	svc := smokeService(fmt.Sprintf("smoke-%d", time.Now().Unix()))
	nodes := []*v1.Node{smokeNode(env.nodeIP)}

	// Clean up even when assertions fail.
	t.Cleanup(func() {
		if err := env.lb.EnsureLoadBalancerDeleted(context.Background(), clusterName, svc); err != nil {
			t.Errorf("cleanup failed, cloud resources may be left behind (name prefix %q): %v",
				env.lb.GetLoadBalancerName(context.Background(), clusterName, svc), err)
		}
	})

	// Create.
	status, err := env.lb.EnsureLoadBalancer(ctx, clusterName, svc, nodes)
	if err != nil {
		t.Fatalf("EnsureLoadBalancer failed: %v", err)
	}
	if len(status.Ingress) == 0 || status.Ingress[0].IP == "" {
		t.Fatalf("expected an ingress IP, got %+v", status)
	}
	t.Logf("load balancer provisioned with ingress IP %s", status.Ingress[0].IP)

	// Exists.
	if _, exists, err := env.lb.GetLoadBalancer(ctx, clusterName, svc); err != nil || !exists {
		t.Fatalf("GetLoadBalancer: exists=%v err=%v", exists, err)
	}

	// Idempotency.
	if _, err := env.lb.EnsureLoadBalancer(ctx, clusterName, svc, nodes); err != nil {
		t.Fatalf("repeated EnsureLoadBalancer failed: %v", err)
	}
	t.Log("repeated ensure is idempotent")

	// Member update.
	if err := env.lb.UpdateLoadBalancer(ctx, clusterName, svc, nodes); err != nil {
		t.Fatalf("UpdateLoadBalancer failed: %v", err)
	}
	t.Log("member update succeeded")

	// Delete and verify.
	if err := env.lb.EnsureLoadBalancerDeleted(ctx, clusterName, svc); err != nil {
		t.Fatalf("EnsureLoadBalancerDeleted failed: %v", err)
	}
	if _, exists, err := env.lb.GetLoadBalancer(ctx, clusterName, svc); err != nil || exists {
		t.Fatalf("load balancer still exists after deletion: exists=%v err=%v", exists, err)
	}
	t.Log("load balancer deleted")
}

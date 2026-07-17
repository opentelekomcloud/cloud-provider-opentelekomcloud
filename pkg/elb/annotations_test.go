package elb

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

func svcWithAnnotations(ann map[string]string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "web",
			Namespace:   "default",
			Annotations: ann,
		},
	}
}

func TestSettingsDefaults(t *testing.T) {
	defaults := config.DefaultLoadBalancerOpts()
	defaults.SubnetID = "subnet-1"
	defaults.AvailabilityZones = []string{"eu-de-01"}

	s, err := settingsForService(defaults, svcWithAnnotations(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SubnetID != "subnet-1" {
		t.Errorf("expected subnet from config, got %q", s.SubnetID)
	}
	if s.LBAlgorithm != "ROUND_ROBIN" {
		t.Errorf("expected default algorithm, got %q", s.LBAlgorithm)
	}
	if !s.HealthCheckEnabled || s.HealthCheckDelay != 5 || s.HealthCheckTimeout != 3 || s.HealthCheckMaxRetries != 3 {
		t.Errorf("unexpected health check defaults: %+v", s)
	}
}

func TestSettingsAnnotationOverrides(t *testing.T) {
	defaults := config.DefaultLoadBalancerOpts()
	defaults.SubnetID = "subnet-1"

	s, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationID:                    "lb-1",
		AnnotationSubnetID:              "subnet-2",
		AnnotationVpcID:                 "vpc-2",
		AnnotationAvailabilityZones:     "eu-de-01, eu-de-02",
		AnnotationL4FlavorID:            "flavor-1",
		AnnotationLBAlgorithm:           "LEAST_CONNECTIONS",
		AnnotationEipIDs:                "eip-1,eip-2",
		AnnotationHealthCheckEnabled:    "false",
		AnnotationHealthCheckDelay:      "10",
		AnnotationHealthCheckTimeout:    "9",
		AnnotationHealthCheckMaxRetries: "7",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.LoadBalancerID != "lb-1" || s.SubnetID != "subnet-2" || s.VpcID != "vpc-2" || s.L4FlavorID != "flavor-1" {
		t.Errorf("unexpected settings: %+v", s)
	}
	if !reflect.DeepEqual(s.AvailabilityZones, []string{"eu-de-01", "eu-de-02"}) {
		t.Errorf("unexpected AZs: %v", s.AvailabilityZones)
	}
	if !reflect.DeepEqual(s.EipIDs, []string{"eip-1", "eip-2"}) {
		t.Errorf("unexpected EIP IDs: %v", s.EipIDs)
	}
	if s.LBAlgorithm != "LEAST_CONNECTIONS" {
		t.Errorf("unexpected algorithm: %q", s.LBAlgorithm)
	}
	if s.HealthCheckEnabled || s.HealthCheckDelay != 10 || s.HealthCheckTimeout != 9 || s.HealthCheckMaxRetries != 7 {
		t.Errorf("unexpected health check settings: %+v", s)
	}
}

func TestSettingsInvalidAnnotations(t *testing.T) {
	defaults := config.DefaultLoadBalancerOpts()

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckEnabled: "not-a-bool",
	})); err == nil {
		t.Error("expected error for invalid bool annotation")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckDelay: "not-an-int",
	})); err == nil {
		t.Error("expected error for invalid int annotation")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckProtocol: "GOPHER",
	})); err == nil {
		t.Error("expected error for invalid health check protocol")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckURLPath: "no-leading-slash",
	})); err == nil {
		t.Error("expected error for URL path without leading slash")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckHTTPMethod: "FETCH",
	})); err == nil {
		t.Error("expected error for invalid HTTP method")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationEipBandwidth: "-5",
	})); err == nil {
		t.Error("expected error for negative EIP bandwidth")
	}

	if _, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationEipIDs:       "eip-1",
		AnnotationEipBandwidth: "100",
	})); err == nil {
		t.Error("expected error for mutually exclusive eip-ids and eip-bandwidth")
	}
}

func TestSettingsEip(t *testing.T) {
	defaults := config.DefaultLoadBalancerOpts()

	s, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationEipBandwidth: "500",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.EipBandwidth != 500 {
		t.Errorf("EipBandwidth = %d, want 500", s.EipBandwidth)
	}
	if s.EipType != "5_bgp" {
		t.Errorf("EipType = %q, want default 5_bgp", s.EipType)
	}

	s, err = settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationEipBandwidth: "10",
		AnnotationEipType:      "5_mailbgp",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.EipType != "5_mailbgp" {
		t.Errorf("EipType = %q, want 5_mailbgp", s.EipType)
	}
}

func TestSettingsHTTPHealthCheck(t *testing.T) {
	defaults := config.DefaultLoadBalancerOpts()

	s, err := settingsForService(defaults, svcWithAnnotations(map[string]string{
		AnnotationHealthCheckProtocol:   "http",
		AnnotationHealthCheckURLPath:    "/healthz",
		AnnotationHealthCheckHTTPMethod: "head",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.HealthCheckProtocol != "HTTP" || s.HealthCheckURLPath != "/healthz" || s.HealthCheckHTTPMethod != "HEAD" {
		t.Errorf("unexpected health check settings: %+v", s)
	}
}

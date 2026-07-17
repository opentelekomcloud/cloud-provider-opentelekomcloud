package elb

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/opentelekomcloud/cloud-provider-opentelekomcloud/pkg/config"
)

// AnnotationPrefix is the common prefix of all Service annotations
// understood by this cloud provider's load balancer implementation.
const AnnotationPrefix = "loadbalancer.opentelekomcloud.com/"

// See docs/loadbalancer.md for the full annotation reference.
const (
	// AnnotationID reuses an existing load balancer; it is never deleted.
	AnnotationID = AnnotationPrefix + "id"

	// AnnotationSubnetID overrides the VIP subnet (neutron subnet ID).
	AnnotationSubnetID = AnnotationPrefix + "subnet-id"

	// AnnotationVpcID overrides the VPC ID.
	AnnotationVpcID = AnnotationPrefix + "vpc-id"

	// AnnotationAvailabilityZones is a comma-separated AZ list.
	AnnotationAvailabilityZones = AnnotationPrefix + "availability-zones"

	// AnnotationL4FlavorID overrides the L4 flavor.
	AnnotationL4FlavorID = AnnotationPrefix + "l4-flavor-id"

	// AnnotationLBAlgorithm overrides the pool algorithm.
	AnnotationLBAlgorithm = AnnotationPrefix + "lb-algorithm"

	// AnnotationEipIDs binds existing EIPs at creation; never released.
	AnnotationEipIDs = AnnotationPrefix + "eip-ids"

	// AnnotationEipBandwidth creates an EIP (Mbit/s) with the LB and
	// releases it on deletion. Mutually exclusive with eip-ids.
	AnnotationEipBandwidth = AnnotationPrefix + "eip-bandwidth"

	// AnnotationEipType is the created EIP network type. Default "5_bgp".
	AnnotationEipType = AnnotationPrefix + "eip-type"

	// AnnotationEipKeep keeps the created EIP on deletion.
	AnnotationEipKeep = AnnotationPrefix + "eip-keep"

	// AnnotationIdleTimeout is the listener idle timeout in seconds.
	AnnotationIdleTimeout = AnnotationPrefix + "idle-timeout"

	// AnnotationHealthCheckEnabled toggles health monitors.
	AnnotationHealthCheckEnabled = AnnotationPrefix + "health-check-enabled"

	// AnnotationHealthCheckDelay is the probe interval in seconds.
	AnnotationHealthCheckDelay = AnnotationPrefix + "health-check-delay"

	// AnnotationHealthCheckTimeout is the probe timeout in seconds.
	AnnotationHealthCheckTimeout = AnnotationPrefix + "health-check-timeout"

	// AnnotationHealthCheckMaxRetries is the probe retry count.
	AnnotationHealthCheckMaxRetries = AnnotationPrefix + "health-check-max-retries"

	// AnnotationHealthCheckProtocol is TCP (default), HTTP or HTTPS;
	// ignored for UDP ports (always UDP_CONNECT).
	AnnotationHealthCheckProtocol = AnnotationPrefix + "health-check-protocol"

	// AnnotationHealthCheckURLPath is the HTTP(S) probe path.
	AnnotationHealthCheckURLPath = AnnotationPrefix + "health-check-url-path"

	// AnnotationHealthCheckHTTPMethod is the HTTP(S) probe method.
	AnnotationHealthCheckHTTPMethod = AnnotationPrefix + "health-check-http-method"
)

// serviceSettings is the effective load balancer configuration for one
// Service: cloud config defaults merged with Service annotations.
type serviceSettings struct {
	LoadBalancerID    string
	SubnetID          string
	VpcID             string
	AvailabilityZones []string
	L4FlavorID        string
	LBAlgorithm       string
	EipIDs            []string
	EipBandwidth      int
	EipType           string
	EipKeep           bool
	IdleTimeout       int

	HealthCheckEnabled    bool
	HealthCheckDelay      int
	HealthCheckTimeout    int
	HealthCheckMaxRetries int
	HealthCheckProtocol   string // "" (TCP), "HTTP" or "HTTPS"
	HealthCheckURLPath    string
	HealthCheckHTTPMethod string
}

// settingsForService merges LoadBalancerOpts defaults with per-Service
// annotation overrides.
func settingsForService(defaults config.LoadBalancerOpts, service *v1.Service) (*serviceSettings, error) {
	s := &serviceSettings{
		SubnetID:              defaults.SubnetID,
		VpcID:                 defaults.VpcID,
		AvailabilityZones:     defaults.AvailabilityZones,
		L4FlavorID:            defaults.L4FlavorID,
		LBAlgorithm:           defaults.LBAlgorithm,
		HealthCheckEnabled:    defaults.HealthCheckEnabled,
		HealthCheckDelay:      defaults.HealthCheckDelay,
		HealthCheckTimeout:    defaults.HealthCheckTimeout,
		HealthCheckMaxRetries: defaults.HealthCheckMaxRetries,
	}
	if s.LBAlgorithm == "" {
		s.LBAlgorithm = "ROUND_ROBIN"
	}

	ann := service.Annotations
	if v := ann[AnnotationID]; v != "" {
		s.LoadBalancerID = v
	}
	if v := ann[AnnotationSubnetID]; v != "" {
		s.SubnetID = v
	}
	if v := ann[AnnotationVpcID]; v != "" {
		s.VpcID = v
	}
	if v := ann[AnnotationAvailabilityZones]; v != "" {
		s.AvailabilityZones = splitTrim(v)
	}
	if v := ann[AnnotationL4FlavorID]; v != "" {
		s.L4FlavorID = v
	}
	if v := ann[AnnotationLBAlgorithm]; v != "" {
		s.LBAlgorithm = v
	}
	if v := ann[AnnotationEipIDs]; v != "" {
		s.EipIDs = splitTrim(v)
	}

	var err error
	if s.EipBandwidth, err = annInt(ann, AnnotationEipBandwidth, 0); err != nil {
		return nil, err
	}
	if s.EipBandwidth < 0 {
		return nil, fmt.Errorf("invalid value %d for annotation %s: must be > 0", s.EipBandwidth, AnnotationEipBandwidth)
	}
	if s.EipBandwidth > 0 && len(s.EipIDs) > 0 {
		return nil, fmt.Errorf("annotations %s and %s are mutually exclusive", AnnotationEipIDs, AnnotationEipBandwidth)
	}
	s.EipType = "5_bgp"
	if v := ann[AnnotationEipType]; v != "" {
		s.EipType = v
	}
	if s.EipKeep, err = annBool(ann, AnnotationEipKeep, false); err != nil {
		return nil, err
	}
	if s.IdleTimeout, err = annInt(ann, AnnotationIdleTimeout, 0); err != nil {
		return nil, err
	}
	if s.IdleTimeout < 0 {
		return nil, fmt.Errorf("invalid value %d for annotation %s: must be >= 0", s.IdleTimeout, AnnotationIdleTimeout)
	}
	if s.HealthCheckEnabled, err = annBool(ann, AnnotationHealthCheckEnabled, s.HealthCheckEnabled); err != nil {
		return nil, err
	}
	if s.HealthCheckDelay, err = annInt(ann, AnnotationHealthCheckDelay, s.HealthCheckDelay); err != nil {
		return nil, err
	}
	if s.HealthCheckTimeout, err = annInt(ann, AnnotationHealthCheckTimeout, s.HealthCheckTimeout); err != nil {
		return nil, err
	}
	if s.HealthCheckMaxRetries, err = annInt(ann, AnnotationHealthCheckMaxRetries, s.HealthCheckMaxRetries); err != nil {
		return nil, err
	}

	if v := ann[AnnotationHealthCheckProtocol]; v != "" {
		proto := strings.ToUpper(v)
		switch proto {
		case "TCP", "HTTP", "HTTPS":
			s.HealthCheckProtocol = proto
		default:
			return nil, fmt.Errorf("invalid value %q for annotation %s: must be TCP, HTTP or HTTPS", v, AnnotationHealthCheckProtocol)
		}
	}
	if v := ann[AnnotationHealthCheckURLPath]; v != "" {
		if !strings.HasPrefix(v, "/") {
			return nil, fmt.Errorf("invalid value %q for annotation %s: must start with /", v, AnnotationHealthCheckURLPath)
		}
		s.HealthCheckURLPath = v
	}
	if v := ann[AnnotationHealthCheckHTTPMethod]; v != "" {
		method := strings.ToUpper(v)
		switch method {
		case "GET", "HEAD", "POST", "PUT", "DELETE", "TRACE", "OPTIONS", "CONNECT", "PATCH":
			s.HealthCheckHTTPMethod = method
		default:
			return nil, fmt.Errorf("invalid value %q for annotation %s: not a valid HTTP method", v, AnnotationHealthCheckHTTPMethod)
		}
	}

	return s, nil
}

func splitTrim(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func annBool(ann map[string]string, key string, def bool) (bool, error) {
	v, ok := ann[key]
	if !ok || v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def, fmt.Errorf("invalid value %q for annotation %s: %w", v, key, err)
	}
	return b, nil
}

func annInt(ann map[string]string, key string, def int) (int, error) {
	v, ok := ann[key]
	if !ok || v == "" {
		return def, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("invalid value %q for annotation %s: %w", v, key, err)
	}
	return i, nil
}

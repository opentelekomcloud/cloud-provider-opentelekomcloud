# Service type LoadBalancer

The cloud controller manager provisions Open Telekom Cloud **ELBv3 (dedicated)
load balancers** for Kubernetes Services of type `LoadBalancer`. For every
Service port it creates a listener, a backend server group (pool), one backend
member per cluster node (targeting the Service NodePort) and, by default, a
health monitor.

## Cloud configuration

Defaults for all Services are set in the `[LoadBalancer]` section of the cloud
config file (the file passed via `--cloud-config`):

```ini
[Global]
auth-url=https://iam.eu-de.otc.t-systems.com/v3
access-key=<AK>
secret-key=<SK>
project-id=<project id>
region=eu-de
domain-name=<domain name>

[LoadBalancer]
# Neutron subnet ID used for the load balancer VIP (required for creation)
subnet-id=00000000-0000-0000-0000-000000000000
# VPC (router) ID the load balancer belongs to
vpc-id=00000000-0000-0000-0000-000000000000
# Availability zones for the dedicated load balancer, repeat for multiple
availability-zone=eu-de-01
availability-zone=eu-de-02
# Optional L4 flavor
#l4-flavor-id=00000000-0000-0000-0000-000000000000
# Pool algorithm: ROUND_ROBIN (default), LEAST_CONNECTIONS or SOURCE_IP
#lb-algorithm=ROUND_ROBIN
# Health monitor defaults
#health-check-enabled=true
#health-check-delay=5
#health-check-timeout=3
#health-check-max-retries=3
```

## Service annotations

Per-Service overrides use the `loadbalancer.opentelekomcloud.com/` prefix:

| Annotation | Description |
|---|---|
| `loadbalancer.opentelekomcloud.com/id` | Reuse an existing load balancer by ID. The provider only manages its own listeners on it and never deletes it. |
| `loadbalancer.opentelekomcloud.com/subnet-id` | VIP subnet override (neutron subnet ID). |
| `loadbalancer.opentelekomcloud.com/vpc-id` | VPC ID override. |
| `loadbalancer.opentelekomcloud.com/availability-zones` | Comma-separated AZ list, e.g. `eu-de-01,eu-de-02`. |
| `loadbalancer.opentelekomcloud.com/l4-flavor-id` | L4 flavor override. |
| `loadbalancer.opentelekomcloud.com/lb-algorithm` | Pool algorithm override. |
| `loadbalancer.opentelekomcloud.com/eip-ids` | Comma-separated list of existing EIP IDs bound to the load balancer at creation. These EIPs are never released by the provider. Mutually exclusive with `eip-bandwidth`. |
| `loadbalancer.opentelekomcloud.com/eip-bandwidth` | Create a new EIP with the given bandwidth (Mbit/s) together with the load balancer and release it again when the Service is deleted. Mutually exclusive with `eip-ids`. |
| `loadbalancer.opentelekomcloud.com/eip-type` | Network type of the EIP created via `eip-bandwidth`. Default `5_bgp`. |
| `loadbalancer.opentelekomcloud.com/eip-keep` | `true` keeps an EIP created via `eip-bandwidth` when the Service is deleted instead of releasing it. Default `false`. |
| `loadbalancer.opentelekomcloud.com/idle-timeout` | Listener keepalive (idle) timeout in seconds. Unset keeps the cloud default. |
| `loadbalancer.opentelekomcloud.com/health-check-enabled` | `true`/`false` – toggle health monitors. |
| `loadbalancer.opentelekomcloud.com/health-check-delay` | Probe interval in seconds. |
| `loadbalancer.opentelekomcloud.com/health-check-timeout` | Probe timeout in seconds. |
| `loadbalancer.opentelekomcloud.com/health-check-max-retries` | Probe retry count. |
| `loadbalancer.opentelekomcloud.com/health-check-protocol` | Probe protocol for TCP ports: `TCP` (default), `HTTP` or `HTTPS`. UDP ports always use `UDP_CONNECT`. |
| `loadbalancer.opentelekomcloud.com/health-check-url-path` | Request path for HTTP/HTTPS probes, must start with `/`. Default `/`. |
| `loadbalancer.opentelekomcloud.com/health-check-http-method` | Request method for HTTP/HTTPS probes. Default `GET`. |

`Service.spec.loadBalancerIP` (deprecated in Kubernetes but still honored) sets
the VIP address at creation time. `Service.spec.sessionAffinity: ClientIP` maps
to `SOURCE_IP` session persistence on the pool, with
`sessionAffinityConfig.clientIP.timeoutSeconds` converted to the ELB
persistence timeout (minutes, clamped to 1–60). Note: once enabled, session
persistence cannot be switched off again on a live pool; recreate the Service
to remove it.

With `externalTrafficPolicy: Local` the health monitor probes the kube-proxy
`healthz` endpoint (`spec.healthCheckNodePort`) over HTTP, so the load
balancer only routes to nodes that host at least one Service endpoint and
client source IPs are preserved. For UDP ports the monitor stays
`UDP_CONNECT` on the member port.

`spec.loadBalancerSourceRanges` is enforced through an ELB IP address group
(whitelist) attached to every listener: only the listed CIDRs may connect.
Clearing the ranges disables the whitelist again.

Only `TCP` and `UDP` ports are supported (ELB L4 listeners).

## Example

```yaml
apiVersion: v1
kind: Service
metadata:
  name: echo
  namespace: default
  annotations:
    loadbalancer.opentelekomcloud.com/health-check-delay: "10"
    # HTTP probe instead of a plain TCP connect check:
    #loadbalancer.opentelekomcloud.com/health-check-protocol: "HTTP"
    #loadbalancer.opentelekomcloud.com/health-check-url-path: "/healthz"
    # Bind an existing EIP so the service is reachable from the internet:
    #loadbalancer.opentelekomcloud.com/eip-ids: "00000000-0000-0000-0000-000000000000"
    # ...or create (and later release) a dedicated EIP with 100 Mbit/s:
    #loadbalancer.opentelekomcloud.com/eip-bandwidth: "100"
spec:
  type: LoadBalancer
  selector:
    app: echo
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 8080
```

After the controller reconciles the Service, `kubectl get service echo` shows
the load balancer VIP (or EIP, if bound) under `EXTERNAL-IP`.

## Lifecycle

- **Create**: a load balancer named `kube_service_<cluster>_<namespace>_<name>`
  is created (unless `.../id` points to an existing one), followed by
  listeners, pools, members and monitors.
- **Node changes**: members are added/removed to match the current node set.
- **Port changes**: listeners owned by the Service that no longer match a
  Service port are removed together with their pool, members and monitor.
- **Delete**: all owned resources are removed; the load balancer itself is
  deleted only if it was created by the provider (no `.../id` annotation).
  An EIP created via `eip-bandwidth` is released as well (OTC only unbinds
  EIPs on load balancer deletion, which would otherwise leave orphaned,
  billed EIPs behind); EIPs supplied via `eip-ids` are left untouched.

Mutating calls are retried on `409 Conflict` (load balancer busy) with
exponential backoff, and the provider waits for the load balancer to return to
`ACTIVE` provisioning status between steps, so transient errors are handled
gracefully by the controller's own retry loop.

## Known limitations

- Only `TCP` and `UDP` Service ports are supported (L4 listeners); no
  TLS termination on the load balancer.
- Session persistence cannot be switched off on a live pool (`ClientIP` →
  `None` requires recreating the Service); enabling and tuning it works.
- With `externalTrafficPolicy: Local`, UDP ports keep a `UDP_CONNECT` probe
  on the member port – nodes without local endpoints are not excluded for
  UDP (TCP ports use the kube-proxy healthz port and are excluded properly).
- The node controllers (Instances API) are not implemented yet: run kubelet
  without `--cloud-provider=external` and the manager with
  `--controllers=service` (the shipped manifests do this).

## Deployment

Ready-to-use manifests live in [`manifests/`](../manifests): RBAC, the
controller Deployment and a `cloud-config` template (create it as a secret in
`kube-system`). Example workloads are in
[`examples/loadbalancer/`](../examples/loadbalancer). A container image can be
built with `make image`.

## Integration tests

`pkg/elb` contains a lifecycle test (`TestLoadBalancerLifecycle`) that runs the
full create → update → delete flow against an in-process fake of the ELBv3 API
(`go test ./pkg/elb/...`). It requires no cloud credentials and runs in CI.

Functional (e2e) tests against a real cluster live in
[`test/e2e/`](../test/e2e) and run via `make test-e2e`; see the README there
for prerequisites.

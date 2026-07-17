# Functional (e2e) tests

These tests run against a **live Kubernetes cluster on Open Telekom Cloud**
with this cloud controller manager deployed. They create a namespace, an nginx
deployment and `LoadBalancer` Services, wait for the load balancer to be
provisioned, optionally verify HTTP reachability, and clean everything up.

They are excluded from normal builds by the `e2e` build tag and never run as
part of `make test` or CI unit tests.

## Prerequisites

1. A Kubernetes cluster running on OTC nodes.
2. The cloud controller manager deployed and healthy, see
   [`manifests/`](../../manifests): the `cloud-config` secret must contain a
   valid `[LoadBalancer]` section (`subnet-id`, `availability-zone`, ...) so
   load balancers can be created without per-Service annotations.
3. `KUBECONFIG` pointing at the cluster (or `~/.kube/config`).
4. Network access from the test runner to the cluster API. The HTTP
   reachability check additionally needs access to the load balancer VIP –
   run the tests from inside the VPC, or set `E2E_EIP_BANDWIDTH` to test via
   a public EIP.

## Running

```sh
make test-e2e
# or directly:
go test -tags e2e ./test/e2e/ -v -timeout 40m
```

## Environment variables

| Variable | Effect |
|---|---|
| `E2E_TIMEOUT` | Per-wait timeout as Go duration (default `10m`). |
| `E2E_SUBNET_ID` | Adds the subnet-id annotation to test Services (overrides cloud-config default). |
| `E2E_AVAILABILITY_ZONES` | Adds the availability-zones annotation (comma-separated). |
| `E2E_EIP_BANDWIDTH` | Requests a public EIP with the given bandwidth (Mbit/s) and enables the HTTP reachability check. The EIP is released with the Service. |
| `E2E_CHECK_HTTP` | `true` forces the HTTP check even without an EIP (requires in-VPC runner). |

## Scenarios

- `TestLoadBalancerService` – full lifecycle: provision, (optional) HTTP
  check through the load balancer, add a second port, delete and wait for
  cloud resource cleanup.
- `TestLoadBalancerSessionAffinity` – provisioning with `ClientIP` session
  affinity and timeout mapping.

# Kubernetes Cloud Provider for Open Telekom Cloud

The Open Telekom Cloud Controller Manager (CCM) connects a self-managed
Kubernetes cluster to Open Telekom Cloud APIs. It watches Services of
`type: LoadBalancer` and provisions dedicated ELBv3 load balancers for them,
so applications get a working `EXTERNAL-IP` on OTC just like on any managed
cloud.

See [Cloud Controller Manager Administration](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)
for background on the CCM concept.

## Features

| Feature | Status |
|---|---|
| `Service` type `LoadBalancer` (dedicated ELBv3) | ✅ TCP/UDP listeners, pools, members, health monitors |
| Health checks | ✅ TCP/UDP_CONNECT/HTTP/HTTPS, fully configurable |
| `externalTrafficPolicy: Local` | ✅ via kube-proxy healthz probes |
| Session affinity (`ClientIP`) | ✅ SOURCE_IP persistence incl. timeout |
| `loadBalancerSourceRanges` | ✅ via ELB IP address groups (whitelist) |
| Public access | ✅ bind existing EIPs or auto-create/release one |
| Shared (pre-existing) load balancers | ✅ via the `id` annotation |
| Authentication | ✅ AK/SK and username/password |
| Node controllers (Instances API) | ❌ not yet – run kubelet without `--cloud-provider=external` |
| Routes | ❌ not planned (CNI handles routing) |

The full annotation reference and behavior details are in
[docs/loadbalancer.md](docs/loadbalancer.md); authentication options in
[docs/auth-configuration.md](docs/auth-configuration.md).

## Quick start

1. Fill in [manifests/cloud-config](manifests/cloud-config) (credentials +
   `[LoadBalancer]` subnet/AZ) and create the secret:

   ```sh
   kubectl create secret -n kube-system generic cloud-config \
     --from-file=manifests/cloud-config
   ```

2. Deploy RBAC and the controller manager:

   ```sh
   kubectl apply -f manifests/rbac-cloud-controller-manager.yaml
   kubectl apply -f manifests/cloud-controller-manager.yaml
   ```

3. Create a LoadBalancer Service (see [examples/loadbalancer](examples/loadbalancer)):

   ```sh
   kubectl apply -f examples/loadbalancer/
   kubectl get svc -w   # wait for EXTERNAL-IP
   ```

## Building and testing

```sh
make build       # binary in bin/
make image       # container image
make test        # unit tests (no cloud access needed)
make test-smoke  # lifecycle against the real OTC API, no cluster required
make test-e2e    # functional tests against a live cluster
```

The smoke and e2e tests are opt-in and skip without credentials; see
[test/smoke](test/smoke) and [test/e2e](test/e2e). A disposable single-node
test cluster can be provisioned with [hack/e2e-infra](hack/e2e-infra).

## Compatibility with Kubernetes

| Kubernetes Version | Latest CCM Version |
|--------------------|--------------------|
| v1.31+             | v0.1.0             |

## Support

Any questions feel free to [submit an issue](https://github.com/opentelekomcloud/cloud-provider-opentelekomcloud/issues/new).

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

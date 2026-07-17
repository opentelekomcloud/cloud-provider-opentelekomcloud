# Cloud smoke test

Exercises the load balancer implementation against the **real OTC API**
without needing a Kubernetes cluster: create → verify → idempotent re-ensure
→ member update → delete. Cheap and fast (one dedicated load balancer for a
few minutes); everything it creates is removed at the end, also on failure.

Excluded from normal builds by the `smoke` build tag.

## Credentials

Taken from a standard `clouds.yaml` (searched in `./`,
`~/.config/openstack/`, `/etc/openstack/`, or the path in
`OS_CLIENT_CONFIG_FILE`), merged with `OS_*` environment variables. Select
the entry with `OS_CLOUD` when the file has several clouds.

Both auth methods work – username/password or AK/SK (`ak`/`sk` keys in the
`auth` section of clouds.yaml).

## Required settings

| Variable | Meaning |
|---|---|
| `SMOKE_SUBNET_ID` | Neutron subnet ID for the load balancer VIP. |
| `SMOKE_AVAILABILITY_ZONES` | Comma-separated AZ list, e.g. `eu-de-01`. |
| `SMOKE_NODE_IP` | An IP inside the subnet to register as a backend member (any free IP works; a real ECS IP makes the health check pass too). |
| `SMOKE_VPC_ID` | Optional VPC ID. |
| `SMOKE_EIP_BANDWIDTH` | Optional: also create/release a public EIP (Mbit/s). |
| `OS_CLOUD` | Optional clouds.yaml entry name. |

## Running

```sh
export OS_CLOUD=otc
export SMOKE_SUBNET_ID=<subnet id>
export SMOKE_AVAILABILITY_ZONES=eu-de-01
export SMOKE_NODE_IP=192.168.0.10
make test-smoke
```

Without credentials or settings the test skips instead of failing, so it is
safe to include in any environment.

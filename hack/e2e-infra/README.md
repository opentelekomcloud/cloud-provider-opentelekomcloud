# e2e test stand

Disposable single-node k3s cluster on an OTC ECS instance for running the
functional tests in [`test/e2e`](../../test/e2e). k3s is started with
`--disable=servicelb --disable-cloud-controller --disable=traefik`, so this
cloud controller manager is the only thing handling `type: LoadBalancer`
Services.

Costs while running: one `s3.large.2` ECS + one EIP (a few cents per hour).
`terraform destroy` removes everything.

## Usage

```sh
cd hack/e2e-infra
export OS_CLOUD=<clouds.yaml entry>          # credentials for terraform/tofu
export SSH_ARGS="-i ~/.ssh/<key>"            # if not your default key

cp terraform.tfvars.example terraform.tfvars # adjust subnet/network IDs
terraform init                               # OpenTofu works too
terraform apply

./fetch-kubeconfig.sh                        # waits for k3s, writes ./kubeconfig
./deploy-ccm.sh                              # cloud-config + CCM deploy

KUBECONFIG=$PWD/kubeconfig make -C ../.. test-e2e
# The VIP is VPC-private; from outside test through an EIP:
#   E2E_EIP_BANDWIDTH=10 KUBECONFIG=$PWD/kubeconfig make -C ../.. test-e2e

terraform destroy                            # tear the stand down
```

`deploy-ccm.sh` deploys in one of two modes (`MODE=image|binary`, default
depends on docker availability): `image` builds the container for linux/amd64
and imports it into the node's containerd, deploying the production
manifests; `binary` cross-compiles the manager and runs it on the node as a
systemd service – no docker needed. Credentials for the generated
cloud-config come from `OS_USERNAME`/`OS_PASSWORD` or
`OS_ACCESS_KEY`/`OS_SECRET_KEY`, with a clouds.yaml fallback (PyYAML
required); project scope (`project_name`) is picked up as well.

## Notes

- The node's security group allows SSH (22) and the Kubernetes API (6443)
  from `admin_cidr` (default: everywhere – restrict it in tfvars), plus all
  TCP/UDP from inside the VPC for LB health checks and NodePorts.
- kubelet runs without `--cloud-provider=external`: this provider has no
  Instances API yet, so the CCM only runs the service controller.

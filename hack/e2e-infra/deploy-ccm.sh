#!/usr/bin/env bash
# Builds the cloud controller manager image, imports it into the k3s node's
# containerd, creates the cloud-config secret and deploys the manifests.
#
# Requirements: docker (with buildx), kubectl, terraform outputs available,
# ./kubeconfig (run fetch-kubeconfig.sh first), and AK/SK credentials in
# OS_ACCESS_KEY / OS_SECRET_KEY (or a clouds.yaml entry named by OS_CLOUD
# with ak/sk fields and PyYAML available).
set -euo pipefail
cd "$(dirname "$0")"

TF=$(command -v terraform || command -v tofu)
# Extra ssh options, e.g. SSH_ARGS="-i ~/.ssh/mykey"
SSH="ssh ${SSH_ARGS:-}"
IP=$("${TF}" output -raw public_ip)
SUBNET_ID=$("${TF}" output -raw subnet_id)
VPC_ID=$("${TF}" output -raw vpc_id)
AZ=$("${TF}" output -raw availability_zone)

IMAGE=cloud-provider-opentelekomcloud:e2e
KUBECONFIG_FILE=${KUBECONFIG_FILE:-./kubeconfig}
KUBECTL="kubectl --kubeconfig=${KUBECONFIG_FILE}"

[ -f "${KUBECONFIG_FILE}" ] || ./fetch-kubeconfig.sh

# --- credentials ---
# Prefer username/password with project scope from clouds.yaml; fall back to
# AK/SK from the environment. The generated cloud-config uses whichever is
# available (the provider itself prefers AK/SK when both are set).
if [ -z "${OS_USERNAME:-}" ] && [ -z "${OS_ACCESS_KEY:-}" ]; then
  echo "no credentials in env, reading clouds.yaml (OS_CLOUD=${OS_CLOUD:-})"
  # Every value is shlex.quote()d: an unquoted secret with spaces or shell
  # metacharacters would be mangled by eval and its fragments echoed to stderr.
  eval "$(python3 - <<'EOF'
import os, shlex, yaml
path = os.path.expanduser("~/.config/openstack/clouds.yaml")
cloud = yaml.safe_load(open(path))["clouds"][os.environ["OS_CLOUD"]]
auth = cloud.get("auth", {})
def export(name, value):
    print(f"export {name}={shlex.quote(str(value))}")
if auth.get("username") and auth.get("password"):
    export("OS_USERNAME", auth["username"])
    export("OS_PASSWORD", auth["password"])
else:
    ak = cloud.get("ak") or auth.get("ak")
    sk = cloud.get("sk") or auth.get("sk")
    if not ak or not sk:
        raise SystemExit("no usable credentials in clouds.yaml entry")
    export("OS_ACCESS_KEY", ak)
    export("OS_SECRET_KEY", sk)
if auth.get("project_name"):
    export("OS_PROJECT_NAME", auth["project_name"])
if auth.get("project_id"):
    export("OS_PROJECT_ID", auth["project_id"])
if auth.get("domain_name") or auth.get("user_domain_name"):
    export("OS_DOMAIN_NAME", auth.get("domain_name") or auth["user_domain_name"])
if auth.get("auth_url"):
    export("OS_AUTH_URL", auth["auth_url"])
if cloud.get("region_name"):
    export("OS_REGION_NAME", cloud["region_name"])
EOF
)"
fi

# Endpoint defaults follow the clouds.yaml entry so the cloud-config talks to
# the same cloud the credentials belong to.
AUTH_URL=${AUTH_URL:-${OS_AUTH_URL:-https://iam.eu-de.otc.t-systems.com/v3}}
REGION=${REGION:-${OS_REGION_NAME:-eu-de}}

# --- cloud-config ---
CLOUD_CONFIG=$(mktemp)
trap 'rm -f "${CLOUD_CONFIG}"' EXIT
cat > "${CLOUD_CONFIG}" <<EOF
[Global]
auth-url=${AUTH_URL}
region=${REGION}
username=${OS_USERNAME:-}
password=${OS_PASSWORD:-}
access-key=${OS_ACCESS_KEY:-}
secret-key=${OS_SECRET_KEY:-}
tenant-name=${OS_PROJECT_NAME:-}
project-id=${OS_PROJECT_ID:-}
domain-name=${OS_DOMAIN_NAME:-}

[LoadBalancer]
subnet-id=${SUBNET_ID}
vpc-id=${VPC_ID}
availability-zone=${AZ}
EOF

# MODE=image builds a container image (needs docker) and deploys the
# manifests; MODE=binary cross-compiles the binary and runs it on the node
# as a systemd service. Default: image when docker is usable, else binary.
if [ -z "${MODE:-}" ]; then
  if docker info >/dev/null 2>&1; then MODE=image; else MODE=binary; fi
fi

if [ "${MODE}" = "image" ]; then
  echo ">>> building ${IMAGE} (linux/amd64)"
  docker buildx build --platform linux/amd64 -t "${IMAGE}" --load ../..
  echo ">>> importing image into k3s containerd on ${IP}"
  docker save "${IMAGE}" | ${SSH} "ubuntu@${IP}" 'sudo k3s ctr images import -'

  ${KUBECTL} -n kube-system delete secret cloud-config --ignore-not-found
  ${KUBECTL} -n kube-system create secret generic cloud-config --from-file=cloud-config="${CLOUD_CONFIG}"

  ${KUBECTL} apply -f ../../manifests/rbac-cloud-controller-manager.yaml
  sed "s|image: cloud-provider-opentelekomcloud:latest|image: docker.io/library/${IMAGE}|" \
    ../../manifests/cloud-controller-manager.yaml | ${KUBECTL} apply -f -
  ${KUBECTL} -n kube-system rollout status deployment/otc-cloud-controller-manager --timeout=180s
else
  echo ">>> cross-compiling the controller manager (linux/amd64)"
  (cd ../.. && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o hack/e2e-infra/cloud-provider-opentelekomcloud ./cmd/cloud-controller-manager)

  echo ">>> installing binary and systemd unit on ${IP}"
  scp ${SSH_ARGS:-} ./cloud-provider-opentelekomcloud "ubuntu@${IP}:/tmp/cloud-provider-opentelekomcloud"
  scp ${SSH_ARGS:-} "${CLOUD_CONFIG}" "ubuntu@${IP}:/tmp/cloud-config"
  rm -f ./cloud-provider-opentelekomcloud
  ${SSH} "ubuntu@${IP}" 'sudo bash -s' <<'REMOTE'
set -euo pipefail
install -m 0755 /tmp/cloud-provider-opentelekomcloud /usr/local/bin/cloud-provider-opentelekomcloud
install -m 0600 /tmp/cloud-config /etc/cloud-provider-opentelekomcloud.conf
rm -f /tmp/cloud-provider-opentelekomcloud /tmp/cloud-config
cat > /etc/systemd/system/otc-ccm.service <<'UNIT'
[Unit]
Description=OTC cloud controller manager (e2e stand)
After=k3s.service
[Service]
ExecStart=/usr/local/bin/cloud-provider-opentelekomcloud \
  --cloud-provider=opentelekomcloud \
  --cloud-config=/etc/cloud-provider-opentelekomcloud.conf \
  --kubeconfig=/etc/rancher/k3s/k3s.yaml \
  --controllers=service \
  --leader-elect=false \
  --v=2
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable otc-ccm.service
systemctl restart otc-ccm.service
sleep 3
systemctl is-active otc-ccm.service
REMOTE
fi

echo ">>> cloud controller manager is running; run the tests with:"
echo "    KUBECONFIG=$(pwd)/${KUBECONFIG_FILE#./} make -C ../.. test-e2e"

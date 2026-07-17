#!/usr/bin/env bash
# Fetches the k3s kubeconfig from the test node and rewrites the API address
# to the node's floating IP. Output: ./kubeconfig
set -euo pipefail
cd "$(dirname "$0")"

TF=$(command -v terraform || command -v tofu)
IP=$("${TF}" output -raw public_ip)
# Extra ssh options, e.g. SSH_ARGS="-i ~/.ssh/mykey"
SSH="ssh ${SSH_ARGS:-}"

for i in $(seq 1 30); do
  if ${SSH} -o StrictHostKeyChecking=accept-new -o ConnectTimeout=5 "ubuntu@${IP}" \
    'test -f /etc/rancher/k3s/k3s.yaml' 2>/dev/null; then
    break
  fi
  echo "waiting for k3s on ${IP} (${i}/30)..."
  sleep 10
done

${SSH} -o StrictHostKeyChecking=accept-new "ubuntu@${IP}" 'cat /etc/rancher/k3s/k3s.yaml' \
  | sed "s/127.0.0.1/${IP}/" > kubeconfig
chmod 600 kubeconfig
echo "kubeconfig written to $(pwd)/kubeconfig"

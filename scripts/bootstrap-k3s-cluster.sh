#!/usr/bin/env bash
# Bootstrap the TinyCloud k3s cluster from the admin machine.
set -euo pipefail

SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
K3S_VERSION="${K3S_VERSION:-v1.30.2+k3s1}"
KUBECONFIG_OUT="${KUBECONFIG_OUT:-$HOME/.kube/tinycloud-oci.yaml}"

CONTROL_HOST="${CONTROL_HOST:-ubuntu@150.136.8.120}"
CONTROL_PUBLIC_IP="${CONTROL_PUBLIC_IP:-150.136.8.120}"
CONTROL_PRIVATE_IP="${CONTROL_PRIVATE_IP:-10.0.0.95}"

WORKER1_HOST="${WORKER1_HOST:-ubuntu@132.145.146.113}"
WORKER1_NAME="${WORKER1_NAME:-k3s-worker-1}"
WORKER1_PRIVATE_IP="${WORKER1_PRIVATE_IP:-10.0.0.73}"

WORKER2_HOST="${WORKER2_HOST:-ubuntu@132.145.154.29}"
WORKER2_NAME="${WORKER2_NAME:-k3s-worker-2}"
WORKER2_PRIVATE_IP="${WORKER2_PRIVATE_IP:-10.0.0.229}"
ENABLE_WORKER2="${ENABLE_WORKER2:-0}"

ssh_run() {
  local host="$1"
  shift
  ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$host" "$@"
}

scp_copy() {
  scp -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$@"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

install_server() {
  echo "--- Installing k3s server on $CONTROL_HOST ---"
  ssh_run "$CONTROL_HOST" "curl -sfL https://get.k3s.io | \
    INSTALL_K3S_VERSION='$K3S_VERSION' \
    INSTALL_K3S_EXEC='server --write-kubeconfig-mode 644 --node-name k3s-control --advertise-address $CONTROL_PRIVATE_IP --node-ip $CONTROL_PRIVATE_IP --tls-san $CONTROL_PRIVATE_IP --tls-san $CONTROL_PUBLIC_IP' \
    sh -"
  ssh_run "$CONTROL_HOST" "sudo systemctl enable --now k3s && sudo systemctl is-active --quiet k3s"
}

read_token() {
  ssh_run "$CONTROL_HOST" "sudo cat /var/lib/rancher/k3s/server/node-token"
}

install_agent() {
  local host="$1"
  local node_name="$2"
  local private_ip="$3"

  echo "--- Installing k3s agent on $host as $node_name ---"
  ssh_run "$host" "curl -sfL https://get.k3s.io | \
    K3S_URL='https://$CONTROL_PRIVATE_IP:6443' \
    K3S_TOKEN='$K3S_TOKEN' \
    INSTALL_K3S_VERSION='$K3S_VERSION' \
    INSTALL_K3S_EXEC='agent --node-name $node_name --node-ip $private_ip' \
    sh -"
  ssh_run "$host" "sudo systemctl enable --now k3s-agent && sudo systemctl is-active --quiet k3s-agent"
}

write_local_kubeconfig() {
  local tmpfile

  tmpfile="$(mktemp)"
  scp_copy "$CONTROL_HOST:/etc/rancher/k3s/k3s.yaml" "$tmpfile"
  mkdir -p "$(dirname "$KUBECONFIG_OUT")"
  sed "s/127.0.0.1/$CONTROL_PUBLIC_IP/" "$tmpfile" > "$KUBECONFIG_OUT"
  chmod 600 "$KUBECONFIG_OUT"
  rm -f "$tmpfile"
}

verify_cluster() {
  echo "--- Verifying cluster ---"
  if command -v kubectl >/dev/null 2>&1; then
    kubectl --kubeconfig "$KUBECONFIG_OUT" get nodes -o wide
    kubectl --kubeconfig "$KUBECONFIG_OUT" -n kube-system get pods -o wide
  else
    ssh_run "$CONTROL_HOST" "sudo kubectl get nodes -o wide && sudo kubectl -n kube-system get pods -o wide"
  fi
}

require_cmd ssh
require_cmd scp
require_cmd sed
require_cmd curl

echo "=== TinyCloud k3s bootstrap ==="
echo "Control plane: $CONTROL_HOST ($CONTROL_PRIVATE_IP / $CONTROL_PUBLIC_IP)"
if [[ "$ENABLE_WORKER2" == "1" ]]; then
  echo "Workers: $WORKER1_HOST, $WORKER2_HOST"
else
  echo "Workers: $WORKER1_HOST"
fi
echo "kubeconfig output: $KUBECONFIG_OUT"
echo

install_server
K3S_TOKEN="$(read_token)"
install_agent "$WORKER1_HOST" "$WORKER1_NAME" "$WORKER1_PRIVATE_IP"
if [[ "$ENABLE_WORKER2" == "1" ]]; then
  install_agent "$WORKER2_HOST" "$WORKER2_NAME" "$WORKER2_PRIVATE_IP"
else
  echo "--- Skipping optional second worker ($WORKER2_NAME) ---"
fi
write_local_kubeconfig
verify_cluster

echo
echo "k3s bootstrap complete."
echo "Local kubeconfig: $KUBECONFIG_OUT"
echo "Next: bootstrap Argo CD and cert-manager with ./scripts/bootstrap-gitops.sh"

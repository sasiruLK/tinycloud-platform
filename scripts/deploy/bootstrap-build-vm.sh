#!/usr/bin/env bash
# Bootstrap build-vm (repurposed monitoring-vm) with coordinator + runner.
# Run on the target ARM64 host as root.
set -euo pipefail

BUILD_VM_PRIVATE_IP="${BUILD_VM_PRIVATE_IP:-10.0.0.107}"
OCIR_REGISTRY="${OCIR_REGISTRY:-iad.ocir.io}"
OCIR_NAMESPACE="${OCIR_NAMESPACE:-idzghas4xwzv}"
OCIR_REPO="${OCIR_REPO:-tinycloud}"
STOP_MONITORING="${STOP_MONITORING:-0}"
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

if [[ $EUID -ne 0 ]]; then
  echo "Run as root: sudo $0"
  exit 1
fi

echo "=== TinyCloud build-vm bootstrap ==="

if ! id tinycloud &>/dev/null; then
  useradd -r -m -s /bin/bash tinycloud
fi
usermod -aG docker tinycloud

install -d -o tinycloud -g tinycloud /etc/tinycloud
install -d -o tinycloud -g tinycloud /var/lib/tinycloud-build-coordinator
install -d -o tinycloud -g tinycloud /var/lib/tinycloud-build-runner/work

if [[ -f "$REPO_ROOT/bin/arm64/tinycloud-build-coordinator" ]]; then
  install -m 755 "$REPO_ROOT/bin/arm64/tinycloud-build-coordinator" /usr/local/bin/tinycloud-build-coordinator
  install -m 755 "$REPO_ROOT/bin/arm64/tinycloud-build-runner" /usr/local/bin/tinycloud-build-runner
else
  echo "Binaries not found at $REPO_ROOT/bin/arm64/. Build locally:"
  echo "  ./scripts/deploy/build-binaries.sh"
  exit 1
fi

install -m 644 "$REPO_ROOT/docs/deploy/tinycloud-build-coordinator.service" /etc/systemd/system/
install -m 644 "$REPO_ROOT/docs/deploy/tinycloud-build-runner.service" /etc/systemd/system/

if [[ ! -f /etc/tinycloud/build-coordinator.env ]]; then
  cp "$REPO_ROOT/docs/deploy/build-coordinator.env.example" /etc/tinycloud/build-coordinator.env
  chown tinycloud:tinycloud /etc/tinycloud/build-coordinator.env
  chmod 600 /etc/tinycloud/build-coordinator.env
  echo "Edit /etc/tinycloud/build-coordinator.env with Vault secrets"
fi

if [[ ! -f /etc/tinycloud/build-runner.env ]]; then
  cp "$REPO_ROOT/docs/deploy/build-runner.env.example" /etc/tinycloud/build-runner.env
  sed -i "s|BUILD_COORDINATOR_URL=.*|BUILD_COORDINATOR_URL=http://127.0.0.1:8090|" /etc/tinycloud/build-runner.env
  sed -i "s|IMAGE_PREFIX=.*|IMAGE_PREFIX=${OCIR_REGISTRY}/${OCIR_NAMESPACE}/${OCIR_REPO}|" /etc/tinycloud/build-runner.env
  sed -i "s|BUILD_CACHE_REF=.*|BUILD_CACHE_REF=${OCIR_REGISTRY}/${OCIR_NAMESPACE}/${OCIR_REPO}/cache:buildkit|" /etc/tinycloud/build-runner.env
  chown tinycloud:tinycloud /etc/tinycloud/build-runner.env
  chmod 600 /etc/tinycloud/build-runner.env
  echo "Edit /etc/tinycloud/build-runner.env with Vault secrets"
fi

docker buildx create --use --name tinycloud 2>/dev/null || docker buildx use tinycloud

if [[ "$STOP_MONITORING" == "1" ]]; then
  echo "Stopping monitoring Docker stack..."
  if [[ -d /opt/monitoring ]]; then
    (cd /opt/monitoring && docker compose down) || true
  fi
  # Common layout from CHAT_LOG_3
  if [[ -f ~/monitoring/docker-compose.yml ]]; then
    (cd ~/monitoring && docker compose down) || true
  fi
fi

systemctl daemon-reload
systemctl enable tinycloud-build-coordinator tinycloud-build-runner
systemctl restart tinycloud-build-coordinator tinycloud-build-runner

echo
echo "=== Bootstrap complete ==="
echo "Coordinator: http://${BUILD_VM_PRIVATE_IP}:8090"
echo "Update gitops-lab apps/tinycloud-api/deployment.yaml BUILD_COORDINATOR_URL"
echo "Next: docker login ${OCIR_REGISTRY}, migrate SQLite from 1GB-vm-2, verify build"

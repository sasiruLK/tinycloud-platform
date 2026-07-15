#!/usr/bin/env bash
# Copy build binaries and bootstrap build-vm over SSH.
# For private VMs (no public IP), set BASTION_SESSION_ID to route through OCI Bastion.
set -euo pipefail

BUILD_VM_IP="${BUILD_VM_IP:-10.0.0.73}"
BUILD_VM_USER="${BUILD_VM_USER:-ubuntu}"
KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REMOTE="/opt/tinycloud-bootstrap"

# Build SSH options — use bastion ProxyCommand if BASTION_SESSION_ID is set.
SSH_OPTS=(-i "$KEY" -o StrictHostKeyChecking=no -o ServerAliveInterval=60)
if [[ -n "${BASTION_SESSION_ID:-}" ]]; then
  BASTION_HOST="host.bastion.us-ashburn-1.oci.oraclecloud.com"
  SSH_OPTS+=(-o "ProxyCommand=ssh -W %h:%p -p 22 -i $KEY -o StrictHostKeyChecking=no ${BASTION_SESSION_ID}@${BASTION_HOST}")
fi

BUILD_VM="${BUILD_VM_USER}@${BUILD_VM_IP}"

"$ROOT/scripts/deploy/build-binaries.sh"

echo "Copying to $BUILD_VM (via bastion: ${BASTION_SESSION_ID:-none})..."
ssh "${SSH_OPTS[@]}" "$BUILD_VM" "sudo rm -rf $REMOTE && sudo mkdir -p $REMOTE/bin/arm64 $REMOTE/docs/deploy $REMOTE/scripts/deploy"

scp "${SSH_OPTS[@]}" \
  "$ROOT/bin/arm64/tinycloud-build-coordinator" \
  "$ROOT/bin/arm64/tinycloud-build-runner" \
  "$BUILD_VM:/tmp/"

scp "${SSH_OPTS[@]}" \
  "$ROOT/docs/deploy/tinycloud-build-coordinator.service" \
  "$ROOT/docs/deploy/tinycloud-build-runner.service" \
  "$ROOT/docs/deploy/build-coordinator.env.example" \
  "$ROOT/docs/deploy/build-runner.env.example" \
  "$ROOT/scripts/deploy/bootstrap-build-vm.sh" \
  "$BUILD_VM:/tmp/"

ssh "${SSH_OPTS[@]}" "$BUILD_VM" "
  sudo mv /tmp/tinycloud-build-coordinator /tmp/tinycloud-build-runner $REMOTE/bin/arm64/
  sudo mv /tmp/tinycloud-build-coordinator.service /tmp/tinycloud-build-runner.service \
    /tmp/build-coordinator.env.example /tmp/build-runner.env.example $REMOTE/docs/deploy/
  sudo mv /tmp/bootstrap-build-vm.sh $REMOTE/scripts/deploy/
  sudo chmod +x $REMOTE/scripts/deploy/bootstrap-build-vm.sh
  sudo REPO_ROOT=$REMOTE STOP_MONITORING=0 $REMOTE/scripts/deploy/bootstrap-build-vm.sh
"

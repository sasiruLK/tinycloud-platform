#!/usr/bin/env bash
# Copy build binaries and bootstrap build-vm over SSH.
set -euo pipefail

BUILD_VM="${BUILD_VM:-ubuntu@150.136.96.152}"
KEY="${SSH_KEY:-$HOME/.ssh/ssh-key-2026-05-16.key}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
REMOTE="/opt/tinycloud-bootstrap"

"$ROOT/scripts/deploy/build-binaries.sh"

echo "Copying to $BUILD_VM..."
ssh -i "$KEY" -o StrictHostKeyChecking=no "$BUILD_VM" "sudo rm -rf $REMOTE && sudo mkdir -p $REMOTE/bin/arm64 $REMOTE/docs/deploy $REMOTE/scripts/deploy"

scp -i "$KEY" -o StrictHostKeyChecking=no \
  "$ROOT/bin/arm64/tinycloud-build-coordinator" \
  "$ROOT/bin/arm64/tinycloud-build-runner" \
  "$BUILD_VM:/tmp/"

scp -i "$KEY" -o StrictHostKeyChecking=no \
  "$ROOT/docs/deploy/tinycloud-build-coordinator.service" \
  "$ROOT/docs/deploy/tinycloud-build-runner.service" \
  "$ROOT/docs/deploy/build-coordinator.env.example" \
  "$ROOT/docs/deploy/build-runner.env.example" \
  "$ROOT/scripts/deploy/bootstrap-build-vm.sh" \
  "$BUILD_VM:/tmp/"

ssh -i "$KEY" -o StrictHostKeyChecking=no "$BUILD_VM" "
  sudo mv /tmp/tinycloud-build-coordinator /tmp/tinycloud-build-runner $REMOTE/bin/arm64/
  sudo mv /tmp/tinycloud-build-coordinator.service /tmp/tinycloud-build-runner.service \
    /tmp/build-coordinator.env.example /tmp/build-runner.env.example $REMOTE/docs/deploy/
  sudo mv /tmp/bootstrap-build-vm.sh $REMOTE/scripts/deploy/
  sudo chmod +x $REMOTE/scripts/deploy/bootstrap-build-vm.sh
  sudo REPO_ROOT=$REMOTE STOP_MONITORING=0 $REMOTE/scripts/deploy/bootstrap-build-vm.sh
"

#!/usr/bin/env bash
# Stop the legacy Docker monitoring stack on build-vm (Phase 2 decommission).
#
# OCI Monitoring/Logging/APM replace the self-hosted monitoring stack.
# Run this only after OCI coverage has been verified.
#
# Usage:
#   ./scripts/phase2/stop-build-vm-monitoring.sh
#   DRY_RUN=1 ./scripts/phase2/stop-build-vm-monitoring.sh
set -euo pipefail

BUILD_VM="${BUILD_VM:-ubuntu@150.136.96.152}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
MONITORING_DIR="${MONITORING_DIR:-/opt/monitoring}"
DRY_RUN="${DRY_RUN:-0}"

ssh_cmd() {
  ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$BUILD_VM" "$@"
}

echo "Checking monitoring stack on $BUILD_VM..."

if ! ssh_cmd "test -f ${MONITORING_DIR}/docker-compose.yml"; then
  echo "No docker-compose.yml at ${MONITORING_DIR} — already decommissioned or never installed."
  exit 0
fi

ssh_cmd "cd ${MONITORING_DIR} && docker compose ps --format json" 2>/dev/null | python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    print("No running containers")
    sys.exit(0)
for line in raw.splitlines():
    try:
        c = json.loads(line)
        name = c.get("Name", "?")
        state = c.get("State", "?")
        print("  {}: {}".format(name, state))
    except json.JSONDecodeError:
        pass
' || ssh_cmd "cd ${MONITORING_DIR} && docker compose ps"

if [[ "$DRY_RUN" == "1" ]]; then
  echo "DRY_RUN=1 — would run: docker compose down in ${MONITORING_DIR}"
  exit 0
fi

echo "--- Stopping monitoring stack ---"
ssh_cmd "cd ${MONITORING_DIR} && docker compose down"

echo "--- Verifying ---"
RUNNING=$(ssh_cmd "docker ps --filter name=victoria --filter name=grafana --filter name=loki -q | wc -l" || echo "0")
if [[ "$RUNNING" != "0" ]]; then
  echo "WARNING: $RUNNING monitoring containers still running" >&2
  exit 1
fi

echo "Monitoring stack stopped on build-vm."
echo "VM metrics: OCI Console → Monitoring → Alarms"
echo "Platform logs: OCI Console → Logging"
echo "External checks: OCI Console → APM → Synthetic Monitoring"

#!/usr/bin/env bash
# Phase 2: OCI Monitoring alarms + Notifications topic for TinyCloud infra.
#
# Creates (idempotent where possible):
#   - ONS topic tinycloud-alerts
#   - HTTPS subscription (Discord webhook) when DISCORD_WEBHOOK_URL is set
#   - Compute alarms: CPU > 80%, disk > 80% per instance
#
# Usage:
#   DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/... ./scripts/phase2/setup-oci-monitoring.sh
#   OCI_RUN_HOST=ubuntu@150.136.8.120 ./scripts/phase2/setup-oci-monitoring.sh
set -euo pipefail

REGION="${OCI_REGION:-us-ashburn-1}"
K3S_CONTROL="${K3S_CONTROL:-ubuntu@150.136.8.120}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
OCI_RUN_HOST="${OCI_RUN_HOST:-auto}"
OCI_CMD="${OCI_CMD:-}"
TOPIC_NAME="${TOPIC_NAME:-tinycloud-alerts}"
COMPARTMENT_ID="${COMPARTMENT_ID:-}"
DISCORD_WEBHOOK_URL="${DISCORD_WEBHOOK_URL:-}"

export SUPPRESS_LABEL_WARNING="${SUPPRESS_LABEL_WARNING:-True}"

oci_ok() {
  if [[ -n "$OCI_CMD" ]]; then
    bash -lc "$OCI_CMD os ns get" &>/dev/null
    return
  fi
  oci os ns get &>/dev/null
}

pick_host() {
  if [[ "$OCI_RUN_HOST" != "auto" ]]; then
    echo "$OCI_RUN_HOST"
    return
  fi
  if oci_ok; then
    echo "local"
    return
  fi
  if ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no \
    -i "$SSH_KEY" "$K3S_CONTROL" \
    'test -x /home/ubuntu/bin/oci || command -v oci >/dev/null'; then
    echo "$K3S_CONTROL"
    return
  fi
  echo "none"
}

run_oci() {
  local host="$1"
  shift
  if [[ "$host" == "local" ]]; then
    if [[ -n "$OCI_CMD" ]]; then
      bash -lc "$OCI_CMD $(printf '%q ' "$@")"
    else
      oci "$@"
    fi
  else
    local quoted=()
    local arg
    for arg in "$@"; do quoted+=("$(printf '%q' "$arg")"); done
    ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$host" \
      "export PATH=/home/ubuntu/bin:\$PATH SUPPRESS_LABEL_WARNING=True; oci ${quoted[*]}"
  fi
}

read_oci_config() {
  local host="$1" key="$2"
  if [[ "$host" == "local" ]]; then
    grep "^${key}=" ~/.oci/config | cut -d= -f2 | tr -d ' '
  else
    ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$host" \
      "grep '^${key}=' ~/.oci/config | cut -d= -f2 | tr -d ' '"
  fi
}

HOST=$(pick_host)
if [[ "$HOST" == "none" ]]; then
  echo "ERROR: OCI CLI not available locally or on k3s-control" >&2
  exit 1
fi

echo "Using OCI CLI via: $HOST"
if [[ "$HOST" == "local" && -n "$OCI_CMD" ]]; then
  echo "Local OCI wrapper: $OCI_CMD"
fi

TEN=$(read_oci_config "$HOST" tenancy)
COMP="${COMPARTMENT_ID:-$TEN}"

echo "--- ONS topic: $TOPIC_NAME ---"
TOPIC_ID=$(run_oci "$HOST" ons topic list \
  --compartment-id "$COMP" \
  --all \
  --query "data[?name=='${TOPIC_NAME}'].\"topic-id\" | [0]" \
  --raw-output 2>/dev/null || true)

if [[ -z "$TOPIC_ID" || "$TOPIC_ID" == "null" ]]; then
  TOPIC_ID=$(run_oci "$HOST" ons topic create \
    --compartment-id "$COMP" \
    --name "$TOPIC_NAME" \
    --description "TinyCloud infrastructure alerts" \
    --query 'data."topic-id"' \
    --raw-output)
  echo "Created topic: $TOPIC_ID"
else
  echo "Topic exists: $TOPIC_ID"
fi

if [[ -n "$DISCORD_WEBHOOK_URL" ]]; then
  echo "--- Discord HTTPS subscription ---"
  EXISTING=$(run_oci "$HOST" ons subscription list \
    --compartment-id "$COMP" \
    --topic-id "$TOPIC_ID" \
    --all \
    --query 'length(data)' \
    --raw-output 2>/dev/null || echo "0")
  if [[ "$EXISTING" == "0" ]]; then
    run_oci "$HOST" ons subscription create \
      --compartment-id "$COMP" \
      --topic-id "$TOPIC_ID" \
      --protocol HTTPS \
      --endpoint "$DISCORD_WEBHOOK_URL" >/dev/null
    echo "Created Discord webhook subscription"
  else
    echo "Subscription(s) already exist on topic (skipping create)"
  fi
else
  echo "DISCORD_WEBHOOK_URL not set — skipping subscription (alarms will fire to topic only)"
fi

echo "--- Compute instance alarms ---"
INSTANCES=$(run_oci "$HOST" compute instance list \
  --compartment-id "$COMP" \
  --lifecycle-state RUNNING \
  --all \
  --query 'data[].{id:id, name:"display-name"}' \
  --output json)

INSTANCE_COUNT=$(echo "$INSTANCES" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')
echo "Found $INSTANCE_COUNT running instances"

create_alarm() {
  local name="$1" query="$2" severity="$3"
  local existing
  existing=$(run_oci "$HOST" monitoring alarm list \
    --compartment-id "$COMP" \
    --display-name "$name" \
    --lifecycle-status ACTIVE \
    --query 'length(data)' \
    --raw-output 2>/dev/null || echo "0")
  if [[ "$existing" != "0" ]]; then
    echo "  Alarm exists: $name"
    return
  fi
  run_oci "$HOST" monitoring alarm create \
    --compartment-id "$COMP" \
    --display-name "$name" \
    --metric-compartment-id "$COMP" \
    --namespace "oci_computeagent" \
    --query-text "$query" \
    --severity "$severity" \
    --destinations "[\"$TOPIC_ID\"]" \
    --is-enabled true \
    --pending-duration "PT5M" \
    --repeat-notification-duration "PT1H" \
    --resolution "1m" \
    >/dev/null
  echo "  Created alarm: $name"
}

echo "$INSTANCES" | python3 -c '
import json, sys
instances = json.load(sys.stdin)
for inst in instances:
    iid = inst["id"]
    name = inst["name"].replace(" ", "-").lower()
    print(f"{name}\t{iid}")
' | while IFS=$'\t' read -r iname iid; do
  create_alarm "tinycloud-${iname}-cpu-high" \
    "CpuUtilization[1m]{resourceId=\"${iid}\"}.mean() > 80" \
    "WARNING"
  create_alarm "tinycloud-${iname}-disk-high" \
    "DiskUtilization[1m]{resourceId=\"${iid}\"}.max() > 80" \
    "CRITICAL"
done

cat <<EOF

Done. OCI Monitoring + Notifications configured.

Next steps:
  1. Confirm alarms in OCI Console → Monitoring → Alarms
  2. Test notification: oci ons message publish --topic-id "$TOPIC_ID" --body '{"title":"TinyCloud test"}'
  3. Create Console Dashboards for infra overview (manual in OCI Console)

Topic OCID: $TOPIC_ID
EOF

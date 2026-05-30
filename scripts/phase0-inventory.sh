#!/usr/bin/env bash
# Phase 0: inventory GHCR usage, Object Storage, and VM reachability.
set -euo pipefail

REGION="${OCI_REGION:-us-ashburn-1}"
NAMESPACE="${OCI_OS_NAMESPACE:-idzghas4xwzv}"
BUCKET="${OCI_BACKUP_BUCKET:-tinycloud-backups}"
GHCR_OWNER="${GHCR_OWNER:-sasirulk}"

echo "=== TinyCloud Phase 0 Inventory ==="
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo

declare -A VMS=(
  ["k3s-control"]="150.136.8.120"
  ["k3s-worker-1"]="132.145.146.113"
  ["k3s-worker-2"]="132.145.154.29"
  ["build-vm"]="150.136.96.152"
  ["1GB-vm-1-runner"]="129.153.180.28"
  ["1GB-vm-2-coordinator"]="157.151.214.150"
)

KEY="${SSH_KEY:-$HOME/.ssh/ssh-key-2026-05-16.key}"

echo "--- VM reachability ---"
for name in "${!VMS[@]}"; do
  ip="${VMS[$name]}"
  if ping -c1 -W2 "$ip" &>/dev/null; then
    priv=$(ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no -i "$KEY" "ubuntu@$ip" \
      "ip -4 -o addr show scope global 2>/dev/null | awk '{print \$4}' | head -1 | sed 's#/.*##'" 2>/dev/null || echo "?")
    echo "  OK  $name  public=$ip  private=$priv"
  else
    echo "  FAIL $name  public=$ip"
  fi
done
echo

if command -v oci &>/dev/null && oci os ns get &>/dev/null 2>&1; then
  echo "--- Object Storage ---"
  echo "  Namespace: $NAMESPACE"
  oci os bucket get --namespace-name "$NAMESPACE" --bucket-name "$BUCKET" \
    --query 'data.{name:name,created:time-created}' 2>/dev/null || echo "  Bucket $BUCKET: not found or no access"
  echo "  Objects in $BUCKET (first 20):"
  oci os object list --namespace-name "$NAMESPACE" --bucket-name "$BUCKET" --limit 20 \
    --query 'data[].{name:name,size:size}' 2>/dev/null || true
  echo
  echo "  Run 'oci os object list --all' for full size audit against 20 GB free tier."
else
  echo "--- Object Storage ---"
  echo "  OCI CLI not configured. Set up ~/.oci/config and re-run."
fi
echo

if command -v gh &>/dev/null && gh auth status &>/dev/null 2>&1; then
  echo "--- GHCR packages ($GHCR_OWNER) ---"
  gh api "users/$GHCR_OWNER/packages?package_type=container" --paginate \
    --jq '.[] | "\(.name)\t\(.visibility)"' 2>/dev/null || echo "  gh api failed"
else
  echo "--- GHCR ---"
  echo "  gh CLI not authenticated. Manual check: https://github.com/$GHCR_OWNER?tab=packages"
fi
echo

echo "=== Done. Record results in docs/infrastructure-runbook.md ==="

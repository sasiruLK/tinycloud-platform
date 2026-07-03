#!/usr/bin/env bash
# Phase 0: inventory OCI-free-tier topology, Object Storage, and VM reachability.
set -euo pipefail

REGION="${OCI_REGION:-us-ashburn-1}"
NAMESPACE="${OCI_OS_NAMESPACE:-idzghas4xwzv}"
BUCKET="${OCI_BACKUP_BUCKET:-tinycloud-backups}"
echo "=== TinyCloud Phase 0 Inventory ==="
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo

declare -A VMS=(
  ["k3s-control"]="150.136.8.120"
  ["k3s-worker-1"]="132.145.146.113"
  ["amd-utility-1"]="129.153.180.28"
  ["amd-utility-2"]="157.151.214.150"
)

KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
OCI_CMD="${OCI_CMD:-}"

run_oci() {
  if [[ -n "$OCI_CMD" ]]; then
    bash -lc "$OCI_CMD $(printf '%q ' "$@")"
    return
  fi
  if command -v oci &>/dev/null; then
    oci "$@"
    return
  fi
  if command -v docker &>/dev/null; then
    docker run --rm -i -v "$HOME/.oci:/oracle/.oci" oci "$@"
    return
  fi
  return 1
}

echo "--- VM reachability ---"
for name in "${!VMS[@]}"; do
  ip="${VMS[$name]}"
  if ping -c1 -W2 "$ip" &>/dev/null; then
    priv=$(ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no -i "$KEY" "ubuntu@$ip" \
      "ip -4 -o addr show scope global 2>/dev/null | awk '{print \$4}' | head -1 | sed 's#/.*##'" 2>/dev/null || echo "?")
    echo "  OK  $name  public=$ip  private=$priv"
  else
    echo "  FAIL $name  public=$ip"
  fi
done
echo

if run_oci os ns get &>/dev/null 2>&1; then
  echo "--- Object Storage ---"
  echo "  Namespace: $NAMESPACE"
  run_oci os bucket get --namespace-name "$NAMESPACE" --bucket-name "$BUCKET" \
    --query 'data.{name:name,created:time-created}' 2>/dev/null || echo "  Bucket $BUCKET: not found or no access"
  echo "  Objects in $BUCKET (first 20):"
  run_oci os object list --namespace-name "$NAMESPACE" --bucket-name "$BUCKET" --limit 20 \
    --query 'data[].{name:name,size:size}' 2>/dev/null || true
  echo
  echo "  Run 'oci os object list --all' for full size audit against 20 GB free tier."
else
  echo "--- Object Storage ---"
  echo "  OCI CLI not configured. Install oci locally or use the Docker wrapper."
fi
echo

echo "--- Target topology ---"
echo "  ARM: 2-node k3s cluster"
echo "  AMD: 2 micro VMs reserved for utility/spare work only"
echo "  Access: OCI Bastion for admin SSH; no dedicated bastion VM"
echo

echo "=== Done. Record results in docs/infrastructure-runbook.md ==="

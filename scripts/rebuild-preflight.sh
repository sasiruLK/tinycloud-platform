#!/usr/bin/env bash
# Fast preflight before any destructive OCI lab rebuild action.
set -euo pipefail

SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
OCI_CMD="${OCI_CMD:-}"
OCI_REGION="${OCI_REGION:-us-ashburn-1}"
BASTION_ID="${BASTION_ID:-ocid1.bastion.oc1.iad.amaaaaaaul44qqiax2v6kabqowojtterbpevcp2yviv7ipf6daot3qhnt42a}"

declare -A HOSTS=(
  ["k3s-control"]="150.136.8.120"
  ["k3s-worker-1"]="132.145.146.113"
  ["amd-utility-1"]="129.153.180.28"
)

run_oci() {
  if [[ -n "$OCI_CMD" ]]; then
    bash -lc "$OCI_CMD $(printf '%q ' "$@")"
    return
  fi
  if command -v oci >/dev/null 2>&1; then
    oci "$@"
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    docker run --rm -i -v "$HOME/.oci:/oracle/.oci" oci "$@"
    return
  fi
  return 1
}

echo "=== TinyCloud rebuild preflight ==="
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "Region: $OCI_REGION"
echo

echo "--- Admin machine ---"
if [[ -r "$SSH_KEY" ]]; then
  echo "SSH key: OK ($SSH_KEY)"
else
  echo "SSH key: FAIL ($SSH_KEY missing)"
fi

if run_oci os ns get >/dev/null 2>&1; then
  echo "OCI auth: OK"
else
  echo "OCI auth: FAIL"
fi
echo

echo "--- SSH reachability ---"
for name in "${!HOSTS[@]}"; do
  ip="${HOSTS[$name]}"
  if out=$(ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=no -i "$SSH_KEY" "ubuntu@$ip" 'hostname' 2>/dev/null); then
    echo "OK    $name  $ip  host=$out"
  else
    echo "FAIL  $name  $ip"
  fi
done
echo

echo "--- Bastion ---"
if state=$(run_oci bastion bastion get --bastion-id "$BASTION_ID" --query 'data."lifecycle-state"' --raw-output 2>/dev/null); then
  echo "Bastion: OK ($state)"
else
  echo "Bastion: FAIL ($BASTION_ID)"
fi
echo

echo "--- Known exception ---"
echo "amd-utility-2 (157.151.214.150): optional utility node; keep it outside the critical path."

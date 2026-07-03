#!/usr/bin/env bash
# Render OCI Vault secrets to env files on a VM.
#
# Requires OCI CLI auth (instance principal on OCI VMs, or API key locally).
# Secrets are written with mode 600; never log secret values.
#
# Usage:
#   VAULT_ID=ocid1.vault... ./scripts/phase2/render-vault-env.sh /etc/tinycloud/coordinator.env coordinator-token build-coordinator-token
#   ./scripts/phase2/render-vault-env.sh --list   # list secret names in vault
#
# Args: <output-file> <env-var-name> <vault-secret-name> [<env-var> <secret>]...
set -euo pipefail

REGION="${OCI_REGION:-us-ashburn-1}"
VAULT_ID="${VAULT_ID:-}"
K3S_CONTROL="${K3S_CONTROL:-ubuntu@150.136.8.120}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
OCI_RUN_HOST="${OCI_RUN_HOST:-auto}"
OCI_CMD="${OCI_CMD:-}"

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

resolve_vault_id() {
  local host="$1"
  if [[ -n "$VAULT_ID" ]]; then
    echo "$VAULT_ID"
    return
  fi
  local ten
  ten=$(read_oci_config "$host" tenancy)
  run_oci "$host" kms management vault list \
    --compartment-id "$ten" \
    --lifecycle-state ACTIVE \
    --all \
    --query 'data[?contains("display-name", `tinycloud`)].id | [0]' \
    --raw-output 2>/dev/null || true
}

fetch_secret_b64() {
  local host="$1" vault_id="$2" secret_name="$3"
  local secret_id
  secret_id=$(run_oci "$host" vault secret list \
    --compartment-id "$(read_oci_config "$host" tenancy)" \
    --vault-id "$vault_id" \
    --name "$secret_name" \
    --lifecycle-state ACTIVE \
    --all \
    --query 'data[0].id' \
    --raw-output 2>/dev/null || true)
  if [[ -z "$secret_id" || "$secret_id" == "null" ]]; then
    echo "ERROR: secret not found: $secret_name" >&2
    return 1
  fi
  run_oci "$host" secrets secret-bundle get \
    --secret-id "$secret_id" \
    --query 'data."secret-bundle-content".content' \
    --raw-output
}

HOST=$(pick_host)
if [[ "$HOST" == "none" ]]; then
  echo "ERROR: OCI CLI not available" >&2
  exit 1
fi
if [[ "$HOST" == "local" && -n "$OCI_CMD" ]]; then
  echo "Using OCI CLI wrapper: $OCI_CMD"
fi

VAULT=$(resolve_vault_id "$HOST")
if [[ -z "$VAULT" || "$VAULT" == "null" ]]; then
  cat <<EOF >&2
ERROR: VAULT_ID not set and no vault matching 'tinycloud' found.

Create vault in OCI Console (KMS → Vaults), then:
  VAULT_ID=ocid1.vault.oc1... ./scripts/phase2/render-vault-env.sh ...

Store secrets with:
  oci vault secret create-base64 --vault-id \$VAULT_ID --secret-name github-token --secret-content-content \$(echo -n 'value' | base64)
EOF
  exit 1
fi

if [[ "${1:-}" == "--list" ]]; then
  run_oci "$HOST" vault secret list \
    --compartment-id "$(read_oci_config "$HOST" tenancy)" \
    --vault-id "$VAULT" \
    --lifecycle-state ACTIVE \
    --all \
    --query 'data[]."secret-name"' \
    --output table
  exit 0
fi

if [[ $# -lt 3 ]]; then
  echo "Usage: $0 <output-file> <ENV_VAR> <vault-secret-name> [<ENV_VAR> <secret> ...]" >&2
  exit 1
fi

OUTFILE="$1"
shift
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

: > "$TMP"
while [[ $# -ge 2 ]]; do
  env_var="$1"
  secret_name="$2"
  shift 2
  b64=$(fetch_secret_b64 "$HOST" "$VAULT" "$secret_name")
  value=$(echo "$b64" | base64 -d)
  printf '%s=%q\n' "$env_var" "$value" >> "$TMP"
  echo "Rendered $env_var from vault secret $secret_name"
done

install -d -m 700 "$(dirname "$OUTFILE")"
install -m 600 "$TMP" "$OUTFILE"
echo "Wrote $(wc -l < "$OUTFILE") vars to $OUTFILE"

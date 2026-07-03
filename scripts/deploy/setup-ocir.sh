#!/usr/bin/env bash
# Create OCIR repo, k8s pull secrets, and optional docker login on build-vm.
#
# Usage:
#   ./scripts/deploy/setup-ocir.sh                    # local OCI CLI (preferred) + SSH to k3s
#   OCI_RUN_HOST=ubuntu@150.136.8.120 ./scripts/...   # force remote OCI CLI on k3s-control
set -euo pipefail

REGION="${OCI_REGION:-us-ashburn-1}"
REGISTRY="${OCIR_REGISTRY:-iad.ocir.io}"
NAMESPACE="${OCIR_NAMESPACE:-idzghas4xwzv}"
REPO="${OCIR_REPO:-tinycloud}"
BUILD_VM="${BUILD_VM:-ubuntu@150.136.96.152}"
K3S_CONTROL="${K3S_CONTROL:-ubuntu@150.136.8.120}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
KUBECTL="${KUBECTL:-sudo kubectl}"
OCI_RUN_HOST="${OCI_RUN_HOST:-auto}"
OCI_CMD="${OCI_CMD:-}"
SKIP_BUILD_VM_LOGIN="${SKIP_BUILD_VM_LOGIN:-0}"

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
    # Pass args safely over SSH (avoids breaking JMESPath quotes in $*)
    local quoted=()
    local arg
    for arg in "$@"; do quoted+=("$(printf '%q' "$arg")"); done
    ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$host" \
      "export PATH=/home/ubuntu/bin:\$PATH SUPPRESS_LABEL_WARNING=True; oci ${quoted[*]}"
  fi
}

repo_exists() {
  local host="$1" ten="$2" name="$3"
  run_oci "$host" artifacts container repository list \
    --compartment-id "$ten" --all \
    --query 'data.items[]."display-name"' --raw-output 2>/dev/null \
    | grep -Fxq "$name"
}

list_auth_token_ids() {
  local host="$1" user_id="$2"
  run_oci "$host" iam auth-token list --user-id "$user_id" 2>/dev/null \
    | grep -oE 'ocid1\.credential\.oc1\.\.[a-zA-Z0-9]+' || true
}

run_kubectl() {
  local host="$1"
  shift
  if [[ "$host" == "local" ]]; then
    $KUBECTL "$@"
  else
    local quoted=()
    local arg
    for arg in "$@"; do quoted+=("$(printf '%q' "$arg")"); done
    ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$host" \
      "sudo kubectl ${quoted[*]}"
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
  cat <<EOF
ERROR: OCI CLI authentication failed locally and k3s-control is unreachable.

Local fix:
  1. Verify ~/.oci/config key_file points to a valid private key
  2. Confirm the API key fingerprint matches OCI Console → User → API Keys
  3. Re-run: oci os ns get

Or run on k3s-control (working config there):
  OCI_RUN_HOST=$K3S_CONTROL ./scripts/deploy/setup-ocir.sh
EOF
  exit 1
fi

echo "Using OCI CLI via: $HOST"
if [[ "$HOST" == "local" && -n "$OCI_CMD" ]]; then
  echo "Local OCI wrapper: $OCI_CMD"
fi

TEN=$(read_oci_config "$HOST" tenancy)
USER_ID=$(read_oci_config "$HOST" user)

OCIR_USER="${NAMESPACE}/$(run_oci "$HOST" iam user get --user-id "$USER_ID" --query 'data.name' --raw-output)"

echo "Tenancy namespace: $NAMESPACE"
echo "OCIR username: $OCIR_USER"

echo "--- Ensure OCIR repository ---"
if repo_exists "$HOST" "$TEN" "$REPO"; then
  echo "Repository $REPO already exists"
else
  set +e
  CREATE_OUT=$(run_oci "$HOST" artifacts container repository create \
    --compartment-id "$TEN" \
    --display-name "$REPO" \
    --is-public false \
    --wait-for-state AVAILABLE 2>&1)
  CREATE_RC=$?
  set -e
  if [[ $CREATE_RC -eq 0 ]]; then
    echo "Created repository $REPO"
  elif echo "$CREATE_OUT" | grep -qE 'NAMESPACE_CONFLICT|already exists'; then
    echo "Repository $REPO already exists"
  else
    echo "$CREATE_OUT" >&2
    exit 1
  fi
fi

echo "--- Auth token and k8s ocir-creds ---"
K8S_HOST="$HOST"
[[ "$K8S_HOST" == "local" ]] && K8S_HOST="$K3S_CONTROL"

# Run token + secrets + docker login on k3s-control so the auth token never
# passes through local shell expansion (OCI tokens contain shell metacharacters).
ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$K8S_HOST" bash -s <<REMOTE
set -euo pipefail
export PATH=/home/ubuntu/bin:\$PATH
export SUPPRESS_LABEL_WARNING=True
USER_ID=\$(grep '^user=' ~/.oci/config | cut -d= -f2 | tr -d ' ')
REGISTRY='$REGISTRY'
OCIR_USER='$OCIR_USER'
BUILD_VM_IP='10.0.0.107'

echo "Removing existing auth tokens..."
while read -r token_id; do
  [[ -z "\$token_id" ]] && continue
  echo "  Deleting auth token \$token_id"
  oci iam auth-token delete --user-id "\$USER_ID" --auth-token-id "\$token_id" --force
done < <(oci iam auth-token list --user-id "\$USER_ID" 2>/dev/null | grep -oE 'ocid1\.credential\.oc1\.\.[a-zA-Z0-9]+' | sort -u)

TOKEN=\$(oci iam auth-token create --user-id "\$USER_ID" --description "tinycloud-ocir-\$(date +%Y%m%d)" --query 'data.token' --raw-output)
echo "  Created new auth token (len \${#TOKEN})"
TOKEN_FILE="/tmp/ocir-token-\$\$"
printf '%s' "\$TOKEN" > "\$TOKEN_FILE"
chmod 600 "\$TOKEN_FILE"
export TOKEN_FILE OCIR_USER REGISTRY

python3 <<'PY'
import json, base64, subprocess, os
token = open(os.environ["TOKEN_FILE"]).read()
user = os.environ["OCIR_USER"]
registry = os.environ["REGISTRY"]
auth = base64.b64encode(f"{user}:{token}".encode()).decode()
cfg = {"auths": {registry: {"auth": auth, "username": user, "password": token}}}
path = "/tmp/ocir-dockerconfig.json"
with open(path, "w") as f:
    json.dump(cfg, f)
for ns in ("argocd", "tinycloud"):
    proc = subprocess.run(
        ["sudo", "kubectl", "create", "namespace", ns, "--dry-run=client", "-o", "yaml"],
        check=True, capture_output=True, text=True,
    )
    apply = subprocess.run(["sudo", "kubectl", "apply", "-f", "-"], input=proc.stdout, text=True, capture_output=True)
    if apply.returncode != 0:
        raise SystemExit(apply.stderr)
    proc = subprocess.run(
        ["sudo", "kubectl", "create", "secret", "generic", "ocir-creds",
         f"--from-file=.dockerconfigjson={path}", "-n", ns,
         "--type=kubernetes.io/dockerconfigjson",
         "--dry-run=client", "-o", "yaml"],
        check=True, capture_output=True, text=True,
    )
    apply = subprocess.run(["sudo", "kubectl", "apply", "-f", "-"], input=proc.stdout, text=True, capture_output=True)
    if apply.returncode != 0:
        raise SystemExit(apply.stderr)
    print(f"  applied ocir-creds in {ns}")
os.remove(path)
PY
REMOTE

if [[ "$SKIP_BUILD_VM_LOGIN" == "1" || -z "$BUILD_VM" ]]; then
  echo "Skipping build-vm docker login"
  echo
  echo "Done. Store token in OCI Vault as ocir-auth-token."
  echo "Image prefix: ${REGISTRY}/${NAMESPACE}/${REPO}"
  exit 0
fi

# Copy token file from k3s to build-vm via local jump host and docker login
REMOTE_TOKEN=$(ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$K8S_HOST" \
  'ls -1 /tmp/ocir-token-* 2>/dev/null | tail -1')
TOKEN_FILE=$(mktemp)
chmod 600 "$TOKEN_FILE"
scp -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" \
  "$K8S_HOST:$REMOTE_TOKEN" "$TOKEN_FILE"
ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$K8S_HOST" "rm -f $REMOTE_TOKEN"

BUILD_TOKEN="/tmp/ocir-token-$$"
scp -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" \
  "$TOKEN_FILE" "$BUILD_VM:$BUILD_TOKEN"
rm -f "$TOKEN_FILE"

ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=no -i "$SSH_KEY" "$BUILD_VM" bash -s <<REMOTE
set -euo pipefail
cat '$BUILD_TOKEN' | sudo -u tinycloud docker login '$REGISTRY' -u '$OCIR_USER' --password-stdin
rm -f '$BUILD_TOKEN'
echo "  docker login OK on build-vm"
REMOTE

echo
echo "Done. Store token in OCI Vault as ocir-auth-token."
echo "Image prefix: ${REGISTRY}/${NAMESPACE}/${REPO}"

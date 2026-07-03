#!/usr/bin/env bash
# Verify the TinyCloud OCIR -> GitOps -> Argo CD deployment path.
set -euo pipefail

KUBECONFIG_PATH="${KUBECONFIG_PATH:-$HOME/.kube/tinycloud-oci.yaml}"
GITOPS_ROOT="${GITOPS_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)/gitops-lab}"
PLATFORM_NAMESPACE="${PLATFORM_NAMESPACE:-tinycloud}"
ARGOCD_NAMESPACE="${ARGOCD_NAMESPACE:-argocd}"
PLATFORM_HOST="${PLATFORM_HOST:-tinycloud.sasiru.lk}"
EXPECTED_REGISTRY_PREFIX="${EXPECTED_REGISTRY_PREFIX:-iad.ocir.io/idzghas4xwzv/tinycloud}"
EXPECTED_PLATFORM_TAG="${EXPECTED_PLATFORM_TAG:-f67788cea249eb0b647e80af115bb89a96b7d32e}"
VERIFY_PUBLIC_ENDPOINT="${VERIFY_PUBLIC_ENDPOINT:-1}"
VERIFY_OCIR_IMAGES="${VERIFY_OCIR_IMAGES:-0}"
USER_APP="${USER_APP:-}"

failures=0

section() {
  echo
  echo "=== $* ==="
}

pass() {
  echo "OK   $*"
}

fail() {
  echo "FAIL $*" >&2
  failures=$((failures + 1))
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing command: $1"
    return 1
  fi
}

kubectl_cmd() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" "$@"
}

check_application() {
  local app="$1"
  local sync health revision

  if ! kubectl_cmd -n "$ARGOCD_NAMESPACE" get application "$app" >/dev/null 2>&1; then
    fail "Argo CD application missing: $app"
    return
  fi

  sync="$(kubectl_cmd -n "$ARGOCD_NAMESPACE" get application "$app" -o jsonpath='{.status.sync.status}' 2>/dev/null || true)"
  health="$(kubectl_cmd -n "$ARGOCD_NAMESPACE" get application "$app" -o jsonpath='{.status.health.status}' 2>/dev/null || true)"
  revision="$(kubectl_cmd -n "$ARGOCD_NAMESPACE" get application "$app" -o jsonpath='{.status.sync.revision}' 2>/dev/null || true)"

  if [[ "$sync" == "Synced" && "$health" == "Healthy" ]]; then
    pass "application $app is Synced/Healthy revision=${revision:-unknown}"
  else
    fail "application $app status sync=${sync:-unknown} health=${health:-unknown} revision=${revision:-unknown}"
  fi
}

check_rollout() {
  local namespace="$1"
  local deployment="$2"
  if kubectl_cmd -n "$namespace" rollout status "deployment/$deployment" --timeout=120s; then
    pass "deployment $namespace/$deployment rollout complete"
  else
    fail "deployment $namespace/$deployment rollout not complete"
  fi
}

check_platform_image() {
  local component="$1"
  local deployment="tinycloud-${component}"
  local image expected

  expected="${EXPECTED_REGISTRY_PREFIX}/tinycloud-platform-${component}:${EXPECTED_PLATFORM_TAG}"
  image="$(kubectl_cmd -n "$PLATFORM_NAMESPACE" get deploy "$deployment" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)"

  if [[ "$image" == "$expected" ]]; then
    pass "$deployment image is $image"
  else
    fail "$deployment image is ${image:-missing}, expected $expected"
  fi

  if kubectl_cmd -n "$PLATFORM_NAMESPACE" get deploy "$deployment" -o jsonpath='{.spec.template.spec.imagePullSecrets[*].name}' 2>/dev/null | grep -qw ocir-creds; then
    pass "$deployment uses imagePullSecret ocir-creds"
  else
    fail "$deployment does not use imagePullSecret ocir-creds"
  fi
}

check_rendered_gitops() {
  local app_path="$1"
  local require_ocir="${2:-1}"
  local rendered

  if [[ ! -d "$GITOPS_ROOT/$app_path" ]]; then
    fail "GitOps path missing: $GITOPS_ROOT/$app_path"
    return
  fi

  rendered="$(kubectl kustomize "$GITOPS_ROOT/$app_path")"
  if grep -q 'ghcr.io/sasirulk' <<<"$rendered"; then
    fail "$app_path renders GHCR image references"
  elif grep -q "$EXPECTED_REGISTRY_PREFIX" <<<"$rendered"; then
    pass "$app_path renders OCIR image references"
  elif [[ "$require_ocir" == "0" ]]; then
    pass "$app_path renders without GHCR image references"
  else
    fail "$app_path does not render expected OCIR prefix $EXPECTED_REGISTRY_PREFIX"
  fi
}

check_ocir_image() {
  local image="$1"
  if [[ "$VERIFY_OCIR_IMAGES" != "1" ]]; then
    echo "SKIP OCIR manifest inspect for $image (set VERIFY_OCIR_IMAGES=1 after docker login)"
    return
  fi
  if docker manifest inspect "$image" >/dev/null; then
    pass "OCIR image exists: $image"
  else
    fail "OCIR image inspect failed: $image"
  fi
}

section "Prerequisites"
require_cmd kubectl || true
require_cmd curl || true
if [[ "$VERIFY_OCIR_IMAGES" == "1" ]]; then
  require_cmd docker || true
fi
if [[ ! -f "$KUBECONFIG_PATH" ]]; then
  fail "kubeconfig not found: $KUBECONFIG_PATH"
fi

section "Local GitOps Render"
check_rendered_gitops apps/tinycloud-api
check_rendered_gitops apps/tinycloud-ui
check_rendered_gitops apps/tinycloud-platform 0

section "Cluster Access"
cluster_ready=0
if kubectl_cmd get nodes -o wide; then
  pass "cluster API reachable"
  cluster_ready=1
else
  fail "cluster API not reachable with $KUBECONFIG_PATH"
fi

if [[ "$cluster_ready" == "1" ]]; then
  section "Required Secrets"
  for namespace in "$ARGOCD_NAMESPACE" "$PLATFORM_NAMESPACE"; do
    if kubectl_cmd -n "$namespace" get secret ocir-creds >/dev/null 2>&1; then
      pass "secret $namespace/ocir-creds exists"
    else
      fail "secret $namespace/ocir-creds missing"
    fi
  done

  section "Argo CD Applications"
  check_application tinycloud-platform
  check_application tinycloud-api
  check_application tinycloud-ui
  if [[ -n "$USER_APP" ]]; then
    check_application "$USER_APP"
  fi

  section "Platform Deployments"
  check_rollout "$PLATFORM_NAMESPACE" tinycloud-api
  check_rollout "$PLATFORM_NAMESPACE" tinycloud-ui
  check_platform_image api
  check_platform_image ui
else
  section "Cluster-Dependent Checks"
  echo "SKIP secrets, Argo CD applications, and deployments because cluster API is not reachable"
fi

section "OCIR Images"
check_ocir_image "${EXPECTED_REGISTRY_PREFIX}/tinycloud-platform-api:${EXPECTED_PLATFORM_TAG}"
check_ocir_image "${EXPECTED_REGISTRY_PREFIX}/tinycloud-platform-ui:${EXPECTED_PLATFORM_TAG}"

section "Public Endpoint"
if [[ "$VERIFY_PUBLIC_ENDPOINT" == "1" ]]; then
  if getent hosts "$PLATFORM_HOST" >/dev/null 2>&1; then
    pass "DNS resolves for $PLATFORM_HOST"
  else
    fail "DNS does not resolve for $PLATFORM_HOST"
  fi

  if curl -Ik --max-time 20 "https://${PLATFORM_HOST}/" >/dev/null; then
    pass "https://${PLATFORM_HOST}/ responds"
  else
    fail "https://${PLATFORM_HOST}/ did not respond successfully"
  fi

  if curl -sk --max-time 20 "https://${PLATFORM_HOST}/api/v1/health" | grep -Eq 'ok|healthy|up'; then
    pass "platform API health endpoint responds"
  else
    fail "platform API health endpoint did not return a healthy response"
  fi
else
  echo "SKIP public endpoint checks (VERIFY_PUBLIC_ENDPOINT=0)"
fi

section "Result"
if (( failures > 0 )); then
  echo "$failures verification check(s) failed" >&2
  exit 1
fi

echo "TinyCloud OCIR + Argo CD end-to-end verification passed."

#!/usr/bin/env bash
# Install Argo CD and bootstrap cert-manager plus base TinyCloud GitOps apps.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GITOPS_ROOT="${GITOPS_ROOT:-$REPO_ROOT/../gitops-lab}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-$HOME/.kube/tinycloud-oci.yaml}"
ARGOCD_VERSION="${ARGOCD_VERSION:-v2.13.3}"
APPLY_PLATFORM_APPS="${APPLY_PLATFORM_APPS:-1}"
CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:-}"
CLOUDFLARE_API_TOKEN_FILE="${CLOUDFLARE_API_TOKEN_FILE:-}"

kubectl_cmd() {
  kubectl --kubeconfig "$KUBECONFIG_PATH" "$@"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

wait_deployment() {
  local namespace="$1"
  local deployment="$2"
  kubectl_cmd -n "$namespace" rollout status "deployment/$deployment" --timeout=300s
}

wait_application_healthy() {
  local name="$1"
  local timeout="${2:-600}"
  local start_ts
  local sync_status
  local health_status

  start_ts="$(date +%s)"
  while true; do
    sync_status="$(kubectl_cmd -n argocd get application "$name" -o jsonpath='{.status.sync.status}' 2>/dev/null || true)"
    health_status="$(kubectl_cmd -n argocd get application "$name" -o jsonpath='{.status.health.status}' 2>/dev/null || true)"

    if [[ "$sync_status" == "Synced" && "$health_status" == "Healthy" ]]; then
      echo "Application $name is Synced/Healthy"
      return 0
    fi

    if (( $(date +%s) - start_ts > timeout )); then
      echo "Timed out waiting for application $name (sync=$sync_status health=$health_status)" >&2
      return 1
    fi

    sleep 10
  done
}

require_secret() {
  local namespace="$1"
  local name="$2"
  if ! kubectl_cmd -n "$namespace" get secret "$name" >/dev/null 2>&1; then
    echo "Missing required secret $name in namespace $namespace" >&2
    return 1
  fi
}

create_cloudflare_secret() {
  local token="$CLOUDFLARE_API_TOKEN"

  if [[ -z "$token" && -n "$CLOUDFLARE_API_TOKEN_FILE" ]]; then
    token="$(<"$CLOUDFLARE_API_TOKEN_FILE")"
  fi

  if [[ -z "$token" ]]; then
    echo "Cloudflare API token is required. Set CLOUDFLARE_API_TOKEN or CLOUDFLARE_API_TOKEN_FILE." >&2
    exit 1
  fi

  kubectl_cmd create namespace cert-manager --dry-run=client -o yaml | kubectl_cmd apply -f -
  kubectl_cmd -n cert-manager create secret generic cloudflare-api-token \
    --from-literal=api-token="$token" \
    --dry-run=client -o yaml | kubectl_cmd apply -f -
}

require_cmd kubectl
require_cmd curl

if [[ ! -f "$KUBECONFIG_PATH" ]]; then
  echo "Kubeconfig not found: $KUBECONFIG_PATH" >&2
  exit 1
fi

echo "=== TinyCloud GitOps bootstrap ==="
echo "kubeconfig: $KUBECONFIG_PATH"
echo "gitops root: $GITOPS_ROOT"
echo

kubectl_cmd create namespace argocd --dry-run=client -o yaml | kubectl_cmd apply -f -
kubectl_cmd apply -n argocd -f "https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml"

wait_deployment argocd argocd-server
wait_deployment argocd argocd-repo-server
wait_deployment argocd argocd-applicationset-controller
wait_deployment argocd argocd-notifications-controller

kubectl_cmd apply -f "$GITOPS_ROOT/argocd/cert-manager.yaml"
wait_application_healthy cert-manager
wait_deployment cert-manager cert-manager
wait_deployment cert-manager cert-manager-webhook
wait_deployment cert-manager cert-manager-cainjector

create_cloudflare_secret
kubectl_cmd apply -f "$GITOPS_ROOT/argocd/cluster-issuers.yaml"

if [[ "$APPLY_PLATFORM_APPS" != "1" ]]; then
  echo
  echo "Argo CD and cert-manager are ready."
  echo "Next: run ./scripts/deploy/setup-ocir.sh, then rerun this script with APPLY_PLATFORM_APPS=1."
  exit 0
fi

kubectl_cmd create namespace tinycloud --dry-run=client -o yaml | kubectl_cmd apply -f -
require_secret argocd ocir-creds
require_secret tinycloud ocir-creds

kubectl_cmd apply -f "$GITOPS_ROOT/argocd/tinycloud-platform.yaml"
kubectl_cmd apply -f "$GITOPS_ROOT/argocd/tinycloud-api.yaml"
kubectl_cmd apply -f "$GITOPS_ROOT/argocd/tinycloud-ui.yaml"
kubectl_cmd apply -f "$GITOPS_ROOT/argocd/applicationset-user-apps.yaml"

wait_application_healthy tinycloud-platform
wait_application_healthy tinycloud-api
wait_application_healthy tinycloud-ui

echo
echo "GitOps bootstrap complete."
echo "Next: verify ingress and deploy one sample service through gitops-lab."

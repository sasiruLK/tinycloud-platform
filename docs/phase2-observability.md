# Phase 2: Observability Offload — Execution Guide

**Goal**: Free build-vm capacity by replacing self-hosted monitoring with OCI Always Free managed observability.

## Architecture (target)

```
┌─────────────────────────────────────────────────────────────┐
│ OCI Monitoring + Notifications (free tier)                    │
│  VM/platform metrics → alarms → ONS topic → Discord/email   │
│  OCI Logging: platform-critical logs only                    │
│  OCI APM synthetics: UI, API health, auth path, sample app   │
│  OCI Dashboards: managed operational overview                │
└─────────────────────────────────────────────────────────────┘

build-vm (150.136.96.152): native ARM64 build coordinator + runner
amd-utility-1 / amd-utility-2: optional AMD utility VMs outside the critical path
```

## Checklist

### 2.1 OCI-managed observability
- [ ] Enable OCI Monitoring alarms for all six VMs
- [ ] Ship platform-critical logs to OCI Logging:
  - TinyCloud API
  - build coordinator/runner
  - Argo CD
  - ingress/auth errors
  - backup and external health-check jobs
- [ ] Add OCI APM synthetic checks:
  - TinyCloud UI
  - `/api/v1/health`
  - OAuth-protected ingress reachability
  - at least one deployed user app route
- [ ] Build OCI Console Dashboard for platform overview

### 2.2 OCI Monitoring + Notifications
- [x] ONS topic `tinycloud-alerts` created
- [x] CPU/disk alarms per running instance
- [ ] Run with `DISCORD_WEBHOOK_URL` to add HTTPS subscription
- [ ] Create Console Dashboards in OCI Console (manual)

### 2.3 Decommission self-hosted monitoring
- [ ] Verify OCI Monitoring alarms fire and notify correctly
- [ ] Verify OCI Logging contains platform-critical failure evidence
- [ ] Verify APM synthetics catch UI/API/sample-app outage
- [ ] Remove VictoriaMetrics/Loki/Grafana/Promtail/vmagent/vmalert/Alertmanager manifests from `gitops-lab`
- [x] VictoriaMetrics/Grafana/Loki Docker stopped on build-vm

### 2.4 OCI Vault (incremental)
- [ ] Create vault `tinycloud-secrets` in OCI Console
- [ ] Store secrets (github-token, ocir-auth-token, discord-webhook, etc.)
- [ ] On VMs: `VAULT_ID=... ./scripts/phase2/render-vault-env.sh /etc/tinycloud/coordinator.env TOKEN build-coordinator-token`

### 2.5 OCI Certificates (optional this phase)
- [ ] Create private CA + issue internal certs for coordinator ↔ API mTLS

## Deploy steps

```bash
# 1. OCI alarms (from machine with OCI auth)
cd tinycloud-platform
DISCORD_WEBHOOK_URL='https://discord.com/api/webhooks/...' \
  ./scripts/phase2/setup-oci-monitoring.sh

# 2. Stop legacy Docker monitoring on build-vm after OCI coverage is verified
./scripts/phase2/stop-build-vm-monitoring.sh

# 3. Copy updated backup scripts to k3s-control
scp -F /dev/null -i ~/.ssh/id_ed25519 \
  gitops-lab/scripts/backup/backup.sh \
  gitops-lab/scripts/backup/validate.sh \
  ubuntu@150.136.8.120:~/gitops-lab/scripts/backup/
```

## Rollback

If OCI coverage is incomplete, keep or restore the current GitOps monitoring stack until parity is reached:

```bash
ssh -F /dev/null -i ~/.ssh/id_ed25519 ubuntu@150.136.96.152 'cd /opt/monitoring && docker compose up -d'
# Re-sync gitops-lab monitoring manifests if they have already been removed
```
*** Add File: /home/sasiru/Personal/lab/tinycloud-platform/scripts/rebuild-preflight.sh
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
  ["k3s-worker-2"]="132.145.154.29"
  ["build-vm"]="150.136.96.152"
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
if run_oci bastion bastion get --bastion-id "$BASTION_ID" --query 'data."lifecycle-state"' --raw-output >/tmp/tinycloud-bastion-state 2>/dev/null; then
  echo "Bastion: OK ($(cat /tmp/tinycloud-bastion-state))"
  rm -f /tmp/tinycloud-bastion-state
else
  echo "Bastion: FAIL ($BASTION_ID)"
fi
echo

echo "--- Known exception ---"
echo "amd-utility-2 (157.151.214.150): excluded from preflight because it currently serves a custom sasiru.lk shell and should be rebuilt before reuse."

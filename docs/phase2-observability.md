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
health-monitor / backup-node: AMD utility VMs
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
scp -i ~/.ssh/ssh-key-2026-05-16.key \
  gitops-lab/scripts/backup/backup.sh \
  gitops-lab/scripts/backup/validate.sh \
  ubuntu@150.136.8.120:~/gitops-lab/scripts/backup/
```

## Rollback

If OCI coverage is incomplete, keep or restore the current GitOps monitoring stack until parity is reached:

```bash
ssh ubuntu@150.136.96.152 'cd /opt/monitoring && docker compose up -d'
# Re-sync gitops-lab monitoring manifests if they have already been removed
```

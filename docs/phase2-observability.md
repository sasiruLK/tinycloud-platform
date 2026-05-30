# Phase 2: Observability Offload — Execution Guide

**Goal**: Free build-vm capacity by moving metrics/logs in-cluster and VM alarms to OCI managed services.

## Architecture (hybrid)

```
┌─────────────────────────────────────────────────────────────┐
│ k3s cluster (monitoring namespace)                          │
│  victoria-metrics:8428  ← vmagent, vmalert, backup scripts  │
│  loki:3100              ← promtail                          │
│  node-exporter, kube-state-metrics (unchanged)              │
└─────────────────────────────────────────────────────────────┘
         ▲
         │ NodePort 30428 (backup metric import from k3s-control host)

┌─────────────────────────────────────────────────────────────┐
│ OCI Monitoring + Notifications (free tier)                    │
│  CPU/disk alarms per VM → ONS topic → Discord webhook       │
└─────────────────────────────────────────────────────────────┘

build-vm (150.136.96.152): Docker monitoring stack STOPPED
```

## Checklist

### 2.1 In-cluster metrics + logs
- [x] Deploy VictoriaMetrics + Loki in `monitoring` namespace (`gitops-lab/apps/monitoring-agents/`)
- [x] Repoint vmagent → `http://victoria-metrics.monitoring.svc:8428/api/v1/write`
- [x] Repoint promtail → `http://loki.monitoring.svc:3100/loki/api/v1/push`
- [x] Repoint vmalert → in-cluster VictoriaMetrics
- [x] Backup scripts → `http://127.0.0.1:30428` (NodePort on k3s-control)
- [x] Applied on cluster (manual apply; **push gitops-lab** so ArgoCD keeps sync)
- [x] Pods healthy — `count(up)` = 8 in VictoriaMetrics

### 2.2 OCI Monitoring + Notifications
- [x] ONS topic `tinycloud-alerts` created
- [x] CPU/disk alarms per running instance
- [ ] Run with `DISCORD_WEBHOOK_URL` to add HTTPS subscription
- [ ] Create Console Dashboards in OCI Console (manual)

### 2.3 Decommission build-vm Docker stack
- [x] VictoriaMetrics/Grafana/Loki Docker stopped on build-vm
- [x] build-vm RAM: ~5.2 Gi available (was ~4.8 Gi buff/cache from stack)

### 2.4 OCI Vault (incremental)
- [ ] Create vault `tinycloud-secrets` in OCI Console
- [ ] Store secrets (github-token, ocir-auth-token, discord-webhook, etc.)
- [ ] On VMs: `VAULT_ID=... ./scripts/phase2/render-vault-env.sh /etc/tinycloud/coordinator.env TOKEN build-coordinator-token`

### 2.5 OCI Certificates (optional this phase)
- [ ] Create private CA + issue internal certs for coordinator ↔ API mTLS

## Deploy steps

```bash
# 1. Push gitops-lab (or apply locally)
cd gitops-lab && git push   # ArgoCD auto-syncs monitoring-agents

# Or apply directly on k3s-control:
ssh -i ~/.ssh/ssh-key-2026-05-16.key ubuntu@150.136.8.120 \
  'cd gitops-lab && git pull && sudo kubectl apply -k apps/monitoring-agents'

# 2. Verify in-cluster stack
ssh -i ~/.ssh/ssh-key-2026-05-16.key ubuntu@150.136.8.120 \
  'sudo kubectl get pods -n monitoring -w'

# 3. OCI alarms (from machine with OCI auth)
cd tinycloud-platform
DISCORD_WEBHOOK_URL='https://discord.com/api/webhooks/...' \
  ./scripts/phase2/setup-oci-monitoring.sh

# 4. Stop legacy Docker monitoring on build-vm
./scripts/phase2/stop-build-vm-monitoring.sh

# 5. Copy updated backup scripts to k3s-control
scp -i ~/.ssh/ssh-key-2026-05-16.key \
  gitops-lab/scripts/backup/backup.sh \
  gitops-lab/scripts/backup/validate.sh \
  ubuntu@150.136.8.120:~/gitops-lab/scripts/backup/
```

## Query metrics (debug)

```bash
# Port-forward VictoriaMetrics UI
kubectl port-forward -n monitoring svc/victoria-metrics 8428:8428

# Sample query
curl 'http://127.0.0.1:8428/api/v1/query?query=up'
```

## Rollback

If in-cluster stack fails, temporarily restore build-vm Docker:

```bash
ssh ubuntu@150.136.96.152 'cd /opt/monitoring && docker compose up -d'
# Revert gitops agent URLs to 150.136.96.152 and re-sync
```

## Resource budget

| Component | Memory limit | Notes |
|-----------|--------------|-------|
| victoria-metrics | 768Mi | emptyDir 2Gi, 14d retention |
| loki | 512Mi | emptyDir 1Gi, 7d retention |
| vmagent + promtail + vmalert | existing | unchanged |

Total added ~1.2GB limits on workers — acceptable on 6GB ARM workers with app headroom.

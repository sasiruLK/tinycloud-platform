# TinyCloud Infrastructure Architecture Plan

## Executive Summary

Current bottlenecks identified:
- **Build time**: 24 min for a simple Go app (QEMU ARM64 emulation on 1GB AMD VMs)
- **Registry**: GHCR free tier = 500MB limit, manual "make public" every time
- **Compute fully allocated**: ARM pool (4 VMs / 24GB) and AMD pool (2 VMs) are both maxed — no room for a new builder VM
- **Monitoring VM blocks build capacity**: A dedicated 6GB ARM VM runs VictoriaMetrics + Grafana + Loki while builds use slow QEMU on AMD
- **No central artifact storage**: SQLite on coordinator, no persistent build cache
- **Most OCI Always Free services unused**: Monitoring, Logging, Notifications, ADB, NoSQL, Vault, LB, Bastions, etc. sit idle

Proposed architecture leverages the **full OCI Always Free catalog** in phases: offload monitoring to free managed services, repurpose the freed ARM VM as a native builder, and adopt remaining free services for data, security, and networking.

**Hard constraint**: Compute is the only maxed pool. Everything else is mostly unused. The lever to unlock native ARM64 builds is to move always-on jobs (monitoring) onto free managed services.

---

## Current State

### VMs

| VM | Public IP | Type | Role | Status |
|----|-----------|------|------|--------|
| k3s-control | 150.136.8.120 | ARM64 (Ampere A1), 6GB | K3s control plane + etcd + API | Active |
| k3s-worker-1 | 132.145.146.113 | ARM64 (Ampere A1), 6GB | App workloads | Active |
| k3s-worker-2 | 132.145.154.29 | ARM64 (Ampere A1), 6GB | App workloads | Active |
| monitoring-vm | 150.136.96.152 | ARM64 (Ampere A1), 6GB | VictoriaMetrics + Grafana + Loki | Active — **to be freed** |
| 1GB-vm-1 | 129.153.180.28 | AMD64 (E2.Micro), 1GB | Image builder (QEMU ARM64) | Active — **to be repurposed** |
| 1GB-vm-2 | 157.151.214.150 | AMD64 (E2.Micro), 1GB | Image builder (QEMU ARM64) | Active — **to be repurposed** |

### Compute allocation (maxed)

| Pool | Limit | Current use | Headroom |
|------|-------|-------------|----------|
| ARM (Ampere A1) | 4 VMs, 24GB RAM, 3,000 OCPU-hrs/mo, 18,000 GB-hrs/mo | 4 VMs × 6GB = 24GB, ~2,976 OCPU-hrs/mo | **None** |
| AMD (E2.Micro) | 2 VMs, 1/8 OCPU, 1GB each | 2 VMs × 1GB | **None** |

Creating a new ARM VM (e.g. "1 VM × 4 OCPU × 24GB") is **not possible** without reshaping or freeing an existing ARM VM.

### Services
- **K3s cluster**: ARM64, Traefik ingress, ArgoCD, cert-manager
- **Monitoring (standalone VM)**: VictoriaMetrics, Grafana, Loki on `monitoring-vm`
- **Platform**: tinycloud-api (Go), tinycloud-ui (React), nginx-proxy, oauth2-proxy
- **Build Pipeline**: Coordinator → Runner (QEMU on AMD) → GHCR → GitOps → ArgoCD
- **Registry**: GHCR (GitHub Container Registry) — free tier limitations

---

## OCI Always Free Catalog → TinyCloud Role Mapping

Every Always Free service and its planned role:

### Compute
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| ARM Compute (Ampere A1) | 4 VMs, 24GB, 3K OCPU-hrs/mo | k3s control + 2 workers + 1 native builder (repurposed from monitoring-vm) |
| AMD Compute (E2.Micro) | 2 VMs, 1GB each | Utility / bastion target / release after build migration |

### Networking
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| Virtual Cloud Networks | 2 VCNs | Primary (us-ashburn-1) + DR region |
| Flexible Network Load Balancer | 1 instance | L4 entry to k3s nodes |
| Load Balancer | 1 instance, 10 Mbps | L7 + public SSL termination in front of Traefik |
| Outbound Data Transfer | 10 TB/month | Ample headroom for image pulls, GitOps, API traffic |
| Service Connector Hub | 2 connectors | Route Logging → Object Storage; Monitoring alarms → Notifications |
| Site-to-Site VPN | 50 IPSec connections | Optional secure home/on-prem → VCN admin access |
| VCN Flow Logs | 10 GB/month (**shared with Logging**) | Security audit, network troubleshooting |

### Observability and Management
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| Monitoring | 500M ingestion, 1B retrieval datapoints | Infra metrics + alarms (replaces standalone VM stack) |
| Logging | 10 GB/month (**shared with Flow Logs**) | Centralized logs (sampled/scoped) |
| Notifications | 1M HTTPS/mo, 1,000 email/mo | Alerts to Discord, email, SMS |
| Application Performance Monitoring | 1,000 traces + 10 synthetic runs/hr | API tracing + synthetic uptime checks on deployed apps |
| Email Delivery | 100 emails/day | Transactional email (build status, invites) |
| Console Dashboards | 100 dashboards | Managed OCI dashboards for infra overview |

### Oracle Databases
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| Autonomous Database | 2 instances total | JSON DB for build history (1); spare for platform metadata (1) |
| NoSQL Database | 133M reads/writes/mo, 25GB/table, 3 tables | Build queue, hot KV, cache |
| HeatWave | 1 instance, 50GB + 50GB backup | Optional analytics / MySQL workload |

### Security
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| Vault | 20 key versions, 150 secrets | All secrets + master encryption keys |
| Certificates | 5 Private CA, 150 TLS certs | Internal mTLS + complement public certs |
| Bastions | 5 instances | SSH to private VMs (remove public IPs) |

### Storage
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| Object Storage (Standard/IA/Archive) | **20 GB total shared** across all tiers | Artifacts, DB backups, cold archive |
| Object Storage API requests | 50,000/month | **Do not use for high-frequency cache sync** |
| OCIR (Container Registry) | Storage draws from **same 20 GB** Object Storage budget | App images + BuildKit registry cache |
| Block Volume | 2 volumes, **200 GB total incl. boot volumes**, 5 backups | k3s persistent volumes |

### Developer Services / Others
| Service | Free limit | TinyCloud role |
|---------|------------|----------------|
| APEX | 744 hrs/instance | Low-code internal admin tools |
| Console Dashboards | 100 dashboards | Managed monitoring views |

### Shared budget warnings
- **Object Storage 20 GB** is shared across Standard, Infrequent Access, Archive, **and OCIR images**
- **Logging 10 GB/month** is shared with VCN Flow Logs
- **Block Volume 200 GB** includes all 6 existing boot volumes — data volumes must fit remaining headroom
- **Object Storage 50,000 API requests/month** — avoid `s3fs` or frequent CLI sync for build cache

---

## Proposed Architecture (Phase-Wise, OCI Always Free Optimized)

### Phase 0: Reality Reconcile (Doc Accuracy)

**Goal**: Align documentation and team understanding with actual deployment and limits.

- [x] Document real 6-VM layout (this document)
- [x] Private IPs and VCN assignments — see [infrastructure-runbook.md](./infrastructure-runbook.md)
- [ ] Inventory current GHCR image sizes and Object Storage usage — run `./scripts/phase0-inventory.sh`

---

### Phase 1: Build Speed & Registry (Week 1)

**Goal**: Fix immediate build pain — native ARM64 builds, OCIR registry, registry-based cache.

#### 1.1 Replace GHCR with OCI Container Registry (OCIR)

**Why**: GHCR free tier = 500MB limit, private-by-default, manual visibility toggles. OCIR is tenancy-scoped, same region as cluster, RBAC-controlled.

**Free tier reality**: OCIR storage draws from the **shared 20 GB Object Storage budget** (Standard/IA/Archive). There is no separate 10 GB registry line in this tenancy's free list.

```
Before: GHCR (500MB limit, private by default)
After:  OCIR (shared 20GB Object Storage budget, tenancy-scoped, RBAC-controlled)
```

**Implementation**:
- Create OCIR repo in `us-ashburn-1`: `iad.ocir.io/<tenancy-namespace>/tinycloud`
- Store OCIR auth token in OCI Vault (Phase 2)
- Update runner to push to `iad.ocir.io/<tenancy>/tinycloud/<app>:<sha>`
- Update k3s with `ocir-creds` image pull secret
- **Prune old tags aggressively** — images + cache compete for the same 20 GB

#### 1.2 Repurpose monitoring-vm as Dedicated Native ARM64 Builder

**Why**: K3s cluster is ARM64. Building on 1GB AMD VMs requires QEMU (24 min for a simple Go app). Native ARM64 build = ~2-3 minutes. No new ARM VM can be created — the pool is maxed at 4 VMs / 24 GB.

**Strategy**: Offload observability from `monitoring-vm` (Phase 2), then repurpose that 6GB ARM64 VM as the native build host.

```
Before: monitoring-vm (6GB ARM) = VictoriaMetrics + Grafana + Loki
        1GB-vm-1/2 (AMD)       = QEMU builders (24 min builds)

After:  build-vm (6GB ARM, same instance) = coordinator + runner (Docker Buildx, native ARM64)
        1GB-vm-1/2 (AMD)       = repurposed (utility/bastion) or released
```

**Implementation**:
- Complete Phase 2 monitoring offload first (OCI coverage proven before deleting self-hosted manifests)
- Install Docker + Docker Buildx on repurposed VM (no QEMU)
- Migrate coordinator DB (SQLite) and runner work dir from AMD VMs
- Repurpose or release the 2 AMD 1GB VMs
- OCPU budget unchanged: still 4 ARM VMs / 24 GB / ~2,976 of 3,000 OCPU-hrs

**Expected improvement**: Build time drops from **24 min → 2-3 min** (10x speedup)

**Fallback** (if monitoring cannot be moved quickly): Run builds as ephemeral in-cluster pods (BuildKit or Kaniko) on `k3s-worker-1/2` with CPU/memory limits and concurrency cap of 1. Less ideal — competes with app workloads on 6GB workers.

#### 1.3 Registry-Based Build Cache (Not Object Storage Sync)

**Why**: Runner rebuilds from scratch every time. Object Storage sync via `s3fs` or frequent CLI sync would blow the **50,000 API requests/month** cap.

**Recommended approach**:
```bash
# BuildKit registry cache stored in OCIR (same shared 20GB budget as images)
docker buildx build \
  --cache-to type=registry,ref=iad.ocir.io/<tenancy>/tinycloud/cache:buildkit \
  --cache-from type=registry,ref=iad.ocir.io/<tenancy>/tinycloud/cache:buildkit \
  ...
```

**Object Storage role** (Phase 3): Artifacts and DB backups only — not hot build cache sync.

**Phase 1 checklist**:
- [x] Create OCIR repos (app images + cache)
- [ ] Offload monitoring from `monitoring-vm` (Phase 2 — in-cluster stack + OCI alarms)
- [x] Repurpose `monitoring-vm` → `build-vm` with coordinator + runner
- [x] Configure BuildKit registry cache in OCIR (`BUILD_CACHE_REF`)
- [x] Update k3s `BUILD_COORDINATOR_URL` + `ocir-creds` in argocd/tinycloud
- [ ] Complete OCIR docker login on build-vm (`scripts/deploy/setup-ocir.sh`)
- [x] Stop AMD runner on 1GB-vm-1
- [ ] End-to-end build + deploy test with OCIR

---

### Phase 2: Observability Offload + Secrets/TLS (Week 2)

**Goal**: Free `monitoring-vm` by moving observability to OCI free managed services. Centralize secrets and TLS.

#### 2.1 Offload Monitoring to OCI Free Tools

**Why**: `monitoring-vm` consumes 1 of 4 ARM VMs (6GB) for a self-hosted stack that OCI provides free and managed.

**Recommended (Full OCI foundation)**:
- **OCI Monitoring** (500M ingestion pts): VM CPU/memory/disk, build metrics, API health
- **OCI Notifications** (1M HTTPS/mo): Alarms → Discord webhook, email
- **OCI Logging** (10 GB/month, **shared with Flow Logs**): platform-critical logs only
- **APM** (1,000 traces + 10 synthetic runs/hr): UI/API/auth/sample-app synthetic checks
- **Console Dashboards** (100): Infra overview without always-on Grafana

Trade-off: weaker Kubernetes pod-metric granularity vs VictoriaMetrics/Loki, but the ARM VM becomes available for native builds and the platform loses a self-hosted operations stack. Keep OCI Logging scoped; do not ship every app pod log.

**Alarms to configure**:
- Build failure rate > 10% in 1 hour → Discord notification
- API pod down > 2 min → Notification
- Disk usage > 80% on any VM → Email
- SSL cert expires in 7 days → Email

#### 2.2 OCI Vault for All Secrets

**Why**: Secrets scattered across K8s Secrets, env files, GitHub Actions. Single source of truth needed.

**OCI Always Free**: 20 key versions + 150 secrets

```
Vault: tinycloud-secrets
├── github-token              # Single PAT for all GitHub operations
├── build-coordinator-token
├── ocir-auth-token           # For pushing/pulling container images
├── ghcr-legacy-token         # Backward compatibility during migration
├── duckdns-token             # DNS updates (until custom domain)
├── discord-webhook           # Alerts
├── argocd-admin
├── db-password               # Autonomous DB password
└── nosql-credentials         # NoSQL API keys
```

**Implementation**:
- VMs read secrets from Vault at boot (OCI CLI + instance principals)
- API pod reads secrets via instance principals
- No secrets in Git, env files, or long-lived K8s Secrets

#### 2.3 OCI Certificates for Internal TLS

**Free tier**: 5 Private CA + 150 private TLS certificates

- Issue internal CA for mTLS between coordinator, runner, and API
- Complement cert-manager for public-facing certs (Phase 4)

**Phase 2 checklist**:
- [ ] Set up OCI Monitoring metrics + alarms (`scripts/phase2/setup-oci-monitoring.sh`)
- [ ] Configure Notifications → Discord/email
- [ ] Configure OCI Logging for platform-critical logs only
- [ ] Create Console Dashboards for infra overview
- [ ] Enable APM synthetic checks for UI/API/auth/sample app
- [ ] Verify OCI coverage, then remove self-hosted monitoring manifests from GitOps
- [x] Decommission VictoriaMetrics/Grafana/Loki Docker on build-vm
- [ ] Migrate all secrets to OCI Vault (`scripts/phase2/render-vault-env.sh`)
- [ ] Issue internal TLS via OCI Certificates

---

### Phase 3: Persistence & Data (Week 3)

**Goal**: Durable build history, fast queue, persistent k3s storage, automated log/metric routing.

#### 3.1 Autonomous JSON Database for Build History

**Why**: Coordinator SQLite is local to the VM. VM dies = build history lost.

**OCI Always Free**: Up to 2 Autonomous Databases — use 1 for builds, keep 1 spare for platform metadata.

```
Database: tinycloud-builds
Type: Autonomous JSON Database
Tier: Always Free
```

**Schema**:
```json
{
  "build_jobs": {
    "id": "uuid",
    "app_name": "string",
    "repo_url": "string",
    "ref": "string",
    "commit_sha": "string",
    "framework": "string",
    "image": "string",
    "tag": "string",
    "status": "queued|running|succeeded|failed",
    "attempts": 1,
    "replicas": 1,
    "port": 8080,
    "env": {},
    "error": "string",
    "created_at": "ISO8601",
    "updated_at": "ISO8601",
    "started_at": "ISO8601",
    "finished_at": "ISO8601"
  },
  "build_logs": {
    "job_id": "uuid",
    "sequence": 1,
    "timestamp": "ISO8601",
    "stream": "stdout|stderr",
    "message": "string"
  }
}
```

#### 3.2 NoSQL Database for Build Queue / Hot KV

**Why**: Fast, low-latency queue operations separate from ADB query workload.

**OCI Always Free**: 133M reads/writes per month, 25 GB per table, up to 3 tables

```
Tables:
  build-queue     # Pending/running job state
  build-cache-kv  # Hot key-value cache
  app-metadata    # App config snapshots
```

#### 3.3 Block Volume for k3s Persistent Storage

**Why**: k3s storage is largely ephemeral. Pod restarts can lose data.

**Free tier reality**: 200 GB **total includes all 6 boot volumes**. Size data volumes to remaining headroom — do not plan 2 × 100 GB data volumes.

```
Data volume (size TBD after boot volume audit):
  - etcd backups
  - PersistentVolumeClaims for apps
  - Container image cache on control node

Large backups → Object Storage (within shared 20 GB) or Archive tier
```

#### 3.4 Object Storage + Archive for Artifacts and Backups

**Within shared 20 GB budget**:
```
tinycloud-artifacts/
  ├── db-backups/         # ADB + NoSQL exports
  ├── gitops-snapshots/   # GitOps repo snapshots
  └── cert-backups/       # Certificate backups

tinycloud-archive/        # Cold backups (same 20 GB pool)
```

#### 3.5 Service Connector Hub

**Free tier**: 2 connectors

```
Connector 1: OCI Logging → Object Storage (log archival within 20 GB)
Connector 2: OCI Monitoring alarm → Notifications (Discord/email)
```

**Phase 3 checklist**:
- [ ] Create Autonomous JSON Database, migrate from SQLite
- [ ] Provision NoSQL tables for build queue
- [ ] Audit boot volume usage, attach sized Block Volume to k3s control
- [ ] Create Object Storage buckets for artifacts/backups
- [ ] Configure Service Connector Hub (2 connectors)
- [ ] Test backup/restore procedures

---

### Phase 4: Edge & Networking (Week 4)

**Goal**: HA ingress, private VMs, security audit, transactional email, public TLS.

#### 4.1 Load Balancers in Front of Traefik

**Why**: DuckDNS is free but unreliable. Single-node Traefik has no HA.

```
Before: DuckDNS → Traefik (single node) → Services
After:  Custom domain → OCI LB (10 Mbps) → Traefik → Services
        Optional: Flexible NLB (L4) for TCP workloads
```

**Free tier**:
- Load Balancer: 1 instance, 10 Mbps (L7, SSL termination)
- Flexible NLB: 1 instance (L4)

#### 4.2 Bastions — Make VMs Private

**Why**: All 6 VMs currently have public IPs. Bastions provide time-limited SSH without exposing VMs.

**Free tier**: 5 Bastions

- Remove public IPs from k3s nodes, build-vm, utility AMD VMs
- SSH via OCI Bastion session only
- Outbound via NAT gateway (within 10 TB/month egress limit)

#### 4.3 VCN Flow Logs

**Free tier**: 10 GB/month — **shared with OCI Logging**

- Enable flow logs on platform subnet only (scoped capture)
- Do not duplicate full log volume already sent to OCI Logging

#### 4.4 Email Delivery + Public TLS

- **Email Delivery** (100/day): Build status notifications, user invites
- **Certificates** + cert-manager: Public TLS for custom domain

**Phase 4 checklist**:
- [ ] Request OCI Load Balancer, configure backend set → Traefik
- [ ] Configure custom domain (migrate off DuckDNS)
- [ ] Create Bastions, remove public IPs from VMs
- [ ] Enable scoped VCN Flow Logs
- [ ] Configure Email Delivery for transactional mail
- [ ] Public TLS via Certificates / cert-manager

---

### Phase 5: Advanced / DR / Internal Tooling (Future)

**Goal**: Optional capabilities when core platform is stable.

#### 5.1 Multi-Region Disaster Recovery

**Free tier**: 2 VCNs across regions

```
Primary: us-ashburn-1 (current)
  - k3s cluster, build pipeline, registry

Standby: us-phoenix-1
  - GitOps repo synced
  - Object Storage cross-region replication
  - Database cross-region backup
  - Documented manual failover
```

#### 5.2 Site-to-Site VPN

**Free tier**: 50 IPSec connections — secure admin access from home/on-prem to VCN without public SSH.

#### 5.3 APEX for Internal Admin Tools

**Free tier**: 744 hrs/instance — low-code dashboards for ops tasks without custom UI development.

#### 5.4 HeatWave (Optional Analytics)

**Free tier**: 1 instance, 50 GB + 50 GB backup — only if analytics/MySQL workload emerges.

**Phase 5 checklist**:
- [ ] Document DR failover runbook
- [ ] Provision 2nd VCN in standby region
- [ ] Configure Site-to-Site VPN (if needed)
- [ ] Evaluate APEX for internal ops dashboard
- [ ] Evaluate HeatWave only if analytics need arises

---

## Implementation Roadmap

### Week 1 — Phase 1: Build & Registry
- [ ] Create OCIR repos (images + BuildKit cache)
- [ ] Begin monitoring offload (Phase 2 prep)
- [ ] Repurpose `monitoring-vm` → native ARM64 `build-vm`
- [ ] Migrate coordinator + runner from AMD VMs
- [ ] Configure BuildKit registry cache
- [ ] Update k3s image pull secrets
- [ ] Repurpose or release AMD 1GB VMs
- [ ] End-to-end build + deploy test

### Week 2 — Phase 2: Observability & Secrets
- [ ] OCI Monitoring + alarms + Notifications
- [ ] Console Dashboards + optional APM
- [ ] Decommission standalone monitoring stack
- [ ] Migrate secrets to OCI Vault
- [ ] Internal TLS via OCI Certificates

### Week 3 — Phase 3: Persistence & Data
- [ ] Autonomous JSON DB + SQLite migration
- [ ] NoSQL build queue
- [ ] Block Volume (sized to headroom after boot volume audit)
- [ ] Object Storage artifacts/backups
- [ ] Service Connector Hub (2 connectors)

### Week 4 — Phase 4: Edge & Security
- [ ] OCI Load Balancer + custom domain
- [ ] Bastions + private VMs
- [ ] VCN Flow Logs (scoped)
- [ ] Email Delivery + public TLS

### Future — Phase 5
- [ ] DR region + 2nd VCN
- [ ] Site-to-Site VPN
- [ ] APEX / HeatWave (as needed)

---

## Cost Analysis (Always Free Only)

| Service | Free Tier Limit | TinyCloud Use | Phase | Status |
|---------|----------------|---------------|-------|--------|
| **Compute ARM** | 4 VMs, 24GB, 3K OCPU-hrs, 18K GB-hrs | k3s control + 2 workers + build-vm | 1 | **4/4 VMs, 24/24 GB — MAXED** |
| **Compute AMD** | 2 VMs, 1GB each | Builders → utility/release | 1 | **2/2 VMs — MAXED** |
| **VCN** | 2 networks | Primary VCN in use | 0 | 1/2 used |
| **Flexible NLB** | 1 instance | L4 entry (optional) | 4 | Unused |
| **Load Balancer** | 1 instance, 10 Mbps | Ingress HA + SSL | 4 | Unused |
| **Outbound Data Transfer** | 10 TB/month | Image pulls, GitOps, API | — | Ample headroom |
| **Service Connector Hub** | 2 connectors | Logging → Storage; Alarms → Notify | 3 | Unused |
| **Site-to-Site VPN** | 50 connections | Admin access | 5 | Unused |
| **VCN Flow Logs** | 10 GB/month (**shared w/ Logging**) | Security audit | 4 | Unused |
| **Monitoring** | 500M ingestion, 1B retrieval | Infra metrics + alarms | 2 | Unused |
| **Logging** | 10 GB/month (**shared w/ Flow Logs**) | Centralized logs | 2 | Unused |
| **Notifications** | 1M HTTPS, 1K email/mo | Alert delivery | 2 | Unused |
| **APM** | 1K traces + 10 synthetic/hr | API tracing + uptime | 2 | Unused |
| **Email Delivery** | 100/day | Transactional email | 4 | Unused |
| **Console Dashboards** | 100 dashboards | Infra overview | 2 | Unused |
| **Autonomous DB** | 2 instances | Build history + platform metadata | 3 | Unused |
| **NoSQL** | 133M r/w, 25GB/table, 3 tables | Build queue / hot KV | 3 | Unused |
| **HeatWave** | 1 instance, 50GB + 50GB backup | Optional analytics | 5 | Unused |
| **Vault** | 20 keys, 150 secrets | All secrets | 2 | Partial (build infra) |
| **Certificates** | 5 CA, 150 TLS certs | Internal + public TLS | 2/4 | Unused |
| **Bastions** | 5 instances | Private VM SSH | 4 | Unused |
| **Object Storage** | **20 GB shared** (std/IA/archive + OCIR) | Images, cache, backups | 1/3 | Partial |
| **Object Storage API** | 50,000 requests/month | Artifacts only (no cache sync) | 1 | — |
| **Block Volume** | 2 vols, **200 GB incl. boot vols**, 5 backups | k3s PVs | 3 | Boot vols in use |
| **Archive Storage** | Part of 20 GB shared pool | Cold backups | 3 | Unused |
| **APEX** | 744 hrs/instance | Internal admin tools | 5 | Unused |

**Total monthly cost**: $0 (Always Free only)

**Shared budgets to watch**:
- Object Storage 20 GB ← OCIR images + BuildKit cache + artifacts + backups + archive
- Logging 10 GB ← OCI Logging + VCN Flow Logs
- Block Volume 200 GB ← 6 boot volumes + data volumes

---

## Decision Log

### 1. Why OCIR over GHCR?
- **GHCR**: 500MB limit, private by default, separate auth per repo, rate limits
- **OCIR**: Tenancy-scoped auth, same region as cluster, no visibility toggle, integrated IAM
- **Caveat**: OCIR storage shares the 20 GB Object Storage budget — prune tags aggressively
- **Verdict**: OCIR wins; manage the shared storage budget carefully

### 2. Why repurpose monitoring-vm instead of creating a new ARM builder?
- **New ARM VM**: Impossible — 4/4 VMs and 24/24 GB already allocated
- **In-cluster builds**: Possible fallback but competes with app workloads on 6GB workers
- **Repurpose monitoring-vm**: Frees compute by offloading observability to OCI free tools; dedicated 6GB native builder
- **Verdict**: Repurpose monitoring-vm; offload monitoring to OCI Monitoring + Notifications + Dashboards

### 3. Why offload monitoring to OCI free tools?
- **Self-hosted VM**: Consumes 1 of 4 ARM VMs (6GB) for always-on Grafana/VictoriaMetrics/Loki
- **OCI managed**: Monitoring, Logging, Notifications, APM, Dashboards are free and don't consume compute
- **Trade-off**: 10 GB Logging cap (shared with Flow Logs); reduced pod-metric granularity vs VictoriaMetrics
- **Verdict**: Full OCI foundation — OCI for observability, platform-critical logs only, and remove self-hosted monitoring after coverage is proven

### 4. Why registry cache over Object Storage sync?
- **Object Storage sync** (`s3fs`, frequent CLI): Blows 50,000 API requests/month cap
- **BuildKit registry cache**: Stored in OCIR, same 20 GB budget as images, no API request penalty
- **Verdict**: Registry-based cache only; Object Storage for artifacts and backups

### 5. Why NoSQL + Autonomous JSON DB (not just one)?
- **NoSQL**: Fast hot-path queue operations, 133M ops/month, low latency
- **Autonomous JSON DB**: Rich build history, queryable logs, managed backups
- **Verdict**: NoSQL for queue/hot KV; ADB for durable history and analytics queries

### 6. Why OCI Vault over K8s Secrets?
- **K8s Secrets**: Base64 encoded (not encrypted by default), etcd storage, manual rotation
- **OCI Vault**: HSM-backed, IAM-controlled, audit logging
- **Verdict**: OCI Vault for all secrets; K8s Secrets only as temporary injection mechanism

### 7. Why Bastions over public IPs?
- **Public IPs**: All 6 VMs exposed to internet scan/brute-force
- **Bastions**: Time-limited, audited SSH; VMs stay private
- **Verdict**: Bastions in Phase 4; remove public IPs once LB provides ingress

---

## Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| No ARM headroom for new VMs | High | Repurpose monitoring-vm; do not plan new ARM VM creation |
| Monitoring migration loses metric fidelity | Medium | Accept lower pod-metric fidelity; keep Kubernetes live inspection for app-level debugging |
| OCI Logging exceeds 10 GB/month (shared w/ Flow Logs) | Medium | Sample/scope logs; avoid duplicating Loki-level verbosity |
| Object Storage 20 GB exhausted (images + cache + backups) | High | Prune OCIR tags; lean images; archive old backups |
| Object Storage 50K API req/mo exceeded | Medium | No s3fs/cache sync; registry-based BuildKit cache only |
| Block Volume 200 GB consumed by boot volumes | High | Audit boot volumes before sizing data volume; backups to Object Storage |
| OCIR auth token expires | High | Vault storage; calendar reminder; rotation procedure |
| JSON DB connection latency | Medium | Connection pooling; local SQLite fallback for active builds during migration |
| Load Balancer misconfiguration | High | Test in staging; document rollback to DuckDNS + Traefik direct |
| Build contention if using in-cluster fallback | Medium | CPU/mem limits; concurrency cap of 1; prefer dedicated build-vm |

---

## Next Steps

1. **Approve this plan** (or suggest phase priority changes)
2. **Start Phase 1**: Set up OCIR, begin monitoring offload, repurpose `monitoring-vm` as `build-vm`
3. **Parallel work**: Create OCIR repos and Vault secrets (requires OCI console access)
4. **Deploy**: Update k3s deployment manifests for new image registry

**Estimated time to full implementation**: 3-4 weeks (part-time)
**Expected build time improvement**: 24 min → 2-3 min (10x)
**Expected reliability improvement**: Eliminates GHCR visibility issues, secret sprawl, monitoring VM waste, and single-VM failure modes

---

*Document version: 2.0*
*Updated: 2026-05-30*
*Author: TinyCloud AI Architect*

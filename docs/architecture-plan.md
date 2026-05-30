# TinyCloud Infrastructure Architecture Plan

## Executive Summary

Current bottlenecks identified:
- **Build time**: 24 min for a simple Go app (QEMU ARM64 emulation on 1GB AMD64)
- **Registry**: GHCR free tier = 500MB limit, manual "make public" every time
- **Compute waste**: 2 AMD64 VMs running when ARM64 native builds would be 10x faster
- **No central artifact storage**: SQLite on coordinator, no persistent build cache

Proposed architecture leverages **OCI Always Free** to eliminate these issues.

---

## Current State

### VMs
| VM | Public IP | Private IP | Type | Role | Status |
|----|-----------|------------|------|------|--------|
| control (k3s) | 150.136.8.120 | 10.0.0.73 | ARM64 (Ampere A1) | K3s master + monitoring + Traefik | Active |
| instance-20260516-1744 | 157.151.214.150 | 10.0.0.55 | AMD64 (E2.Micro) | Build Coordinator | Active |
| instance-20260518-2222 | 129.153.180.28 | 10.0.0.122 | AMD64 (E2.Micro) | Build Runner | Active |

### Services
- **K3s cluster**: ARM64, Traefik ingress, ArgoCD, cert-manager, VictoriaMetrics, Grafana
- **Platform**: tinycloud-api (Go), tinycloud-ui (React), nginx-proxy, oauth2-proxy
- **Build Pipeline**: Coordinator → Runner → GHCR → GitOps → ArgoCD
- **Registry**: GHCR (GitHub Container Registry) — free tier limitations

---

## Proposed Architecture (OCI Always Free Optimized)

### Phase 1: Fix Immediate Pain (Week 1)

#### 1. Replace GHCR with OCI Container Registry (OCIR)

**Why**: OCI provides **10GB free** container registry per tenancy. No per-repo size limit. No "make public" toggle hell.

```
Before: GHCR (500MB limit, private by default)
After:  OCI Container Registry (10GB, tenancy-scoped, RBAC-controlled)
```

**Implementation**:
- Create OCIR repo in `us-ashburn-1`: `iad.ocir.io/<tenancy-namespace>/tinycloud`
- Use OCI Vault to store auth token (not in K8s secrets)
- Update runner to push to `iad.ocir.io/<tenancy>/tinycloud/<app>:<sha>`
- Update k3s with `ocir-creds` image pull secret

**Benefits**:
- No 500MB limit
- No manual visibility changes
- Same network region = faster pull/push
- Integrated with OCI IAM

#### 2. Move Builder to ARM64 VM

**Why**: K3s cluster is ARM64. Building ARM64 images on AMD64 requires QEMU emulation (24 min for a simple Go app). Native ARM64 build = ~2-3 minutes.

**OCI Always Free ARM64 specs**:
- Up to 4 VMs, 24GB RAM total, 3,000 OCPU hours/month
- Example: 1 VM × 4 OCPU × 24GB RAM (single large VM)
- Or: 2 VMs × 2 OCPU × 12GB RAM each
- Or: 4 VMs × 1 OCPU × 6GB RAM each

**Recommended**: 
```
Option A (recommended): 1 VM × 4 OCPU × 24GB RAM = "build-ampere"
  - Runs both coordinator + runner natively
  - Enough RAM for concurrent builds
  - Native ARM64 = no QEMU
  
Option B: 2 VMs × 2 OCPU × 12GB RAM
  - VM 1: Coordinator
  - VM 2: Runner
  - More isolation, but more management overhead
```

**Implementation**:
- Create new ARM64 VM in same VCN (10.0.0.x)
- Install Docker, Docker Buildx (no QEMU needed!)
- Migrate coordinator DB (SQLite) and runner work dir
- Decommission the 2 AMD64 VMs (free up Always Free slots)

**Expected improvement**: Build time drops from **24 min → 2-3 min** (10x speedup)

#### 3. Use OCI Object Storage as Build Cache + Artifact Store

**Why**: Runner rebuilds from scratch every time. No layer caching across builds.

```
Bucket: tinycloud-build-cache (Standard, Always Free 20GB)
  ├── docker-build-cache/     # Docker layer cache
  ├── go-mod-cache/           # Go module cache across builds
  ├── npm-cache/              # npm cache across builds
  └── build-artifacts/        # Build outputs, test reports
```

**Implementation**:
- Mount OCI Object Storage via `s3fs` or OCI CLI sync
- Configure Docker Buildx to use remote cache
- Share Go modules and npm cache across builds

**Benefits**:
- Faster incremental builds
- Survive VM recreation
- No local disk pressure on 1GB/24GB VMs

---

### Phase 2: Enhance Reliability (Week 2)

#### 4. Replace SQLite with OCI Autonomous JSON Database

**Why**: Coordinator SQLite is local to the VM. VM dies = build history lost.

**OCI Always Free**: Up to 2 Autonomous Databases (JSON, ATP, or ADW)

```
Database: tinycloud-builds
Type: Autonomous JSON Database
Storage: 20GB
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

**Benefits**:
- Persistent build history
- Queryable logs (search by app, status, time range)
- Survive VM replacement
- OCI-managed backups

#### 5. OCI Vault for All Secrets

**Why**: Secrets scattered across K8s Secrets, env files, GitHub Actions. Single source of truth needed.

**OCI Always Free**: 20 key versions + 150 secrets

**Vault Structure**:
```
Vault: tinycloud-secrets
├── github-token          # Single PAT for all GitHub operations
├── build-coordinator-token
├── ocir-auth-token       # For pushing/pulling container images
├── ghcr-legacy-token     # Keep for backward compatibility
├── duckdns-token         # For DNS updates
├── discord-webhook       # For alerts
├── argocd-admin          # ArgoCD admin password
├── grafana-admin         # Grafana admin password
└── db-password           # Autonomous DB password
```

**Implementation**:
- API pod reads secrets from OCI Vault via instance principals
- VMs read secrets from Vault at boot (OCI CLI + instance principals)
- No secrets in Git, no secrets in env files, no secrets in K8s

#### 6. OCI Monitoring + Alarms (Replace VictoriaMetrics?)

**Why**: Currently running VictoriaMetrics + Grafana on a separate VM. OCI Monitoring is free and managed.

**OCI Always Free**: 500M ingestion datapoints, 1B retrieval datapoints

**What to monitor**:
- k3s node CPU, memory, disk
- Pod health (via OCI Monitoring Agent or custom metrics)
- Build queue depth, build duration, build failures
- API response times, error rates
- Traefik ingress metrics

**Alarms**:
- Build failure rate > 10% in 1 hour → Discord notification
- API pod down > 2 min → Page
- Disk usage > 80% → Slack
- SSL cert expires in 7 days → Email

**Decision**: Keep VictoriaMetrics for detailed pod metrics (it's already working), but add OCI Monitoring for infrastructure-level metrics and alarms.

---

### Phase 3: Scale & Optimize (Week 3-4)

#### 7. OCI Load Balancer + Custom Domain

**Why**: Currently using DuckDNS (free but unreliable) + single-node Traefik. OCI Load Balancer is free (10 Mbps) and provides HA.

```
Before: DuckDNS → Traefik (single node) → Services
After:  Custom domain → OCI LB → Traefik (ready for multi-node) → Services
```

**Implementation**:
- Request OCI Load Balancer (Always Free tier: 1 instance, 10 Mbps)
- Backend set: k3s worker nodes (when scaled)
- Health check: Traefik /ping endpoint
- SSL termination: At LB (OCI-managed certificates) or at Traefik (cert-manager)

**Custom domain**: Migrate from `*.duckdns.org` to `*.tinycloud.internal` or buy a domain

#### 8. OCI Block Volume for k3s Persistent Storage

**Why**: Currently all k3s storage is ephemeral. Pod restarts lose data.

**OCI Always Free**: 2 block volumes, 200GB total

```
Volume 1: k3s-data (100GB)
  - etcd backups
  - PersistentVolumeClaims for apps
  - Container image cache

Volume 2: platform-backups (100GB)
  - GitOps repo snapshots
  - Build logs (before JSON DB migration)
  - Certificate backups
```

**Implementation**:
- Attach volume to control node
- Configure k3s to use local-path-provisioner with Block Volume backend
- Daily snapshot to Object Storage

#### 9. VCN Flow Logs for Security Auditing

**Why**: No visibility into who is accessing what. Network policies are blind.

**OCI Always Free**: 10GB logs/month

**What to log**:
- Ingress to platform namespace (who accessed what app)
- Egress from API pod (what external services are called)
- Build coordinator → runner traffic
- Cross-VCN traffic

**Benefits**:
- Detect unauthorized access
- Audit trail for compliance
- Debug network issues

---

### Phase 4: Advanced (Future)

#### 10. OCI Functions (Serverless) for Lightweight Operations

**Why**: Some operations don't need a 24/7 running pod.

**Use cases**:
- Webhook receiver (GitHub → trigger build)
- Health check notifier (ping external services, alert on failure)
- Cleanup jobs (prune old builds, old images)
- Certificate renewal checker

**OCI Always Free**: 2M invocations/month

#### 11. OCI API Gateway for External API Exposure

**Why**: Currently exposing raw API via Traefik. API Gateway provides rate limiting, auth, and monitoring.

```
Before: GitHub → Traefik → tinycloud-api
After:  GitHub → OCI API Gateway → Traefik → tinycloud-api
```

**Benefits**:
- Rate limiting (prevent abuse)
- API key management
- Request/response transformation
- Usage analytics

#### 12. Multi-Region Disaster Recovery

**OCI Free**: 2 VCNs across regions

```
Primary: us-ashburn-1 (current)
  - k3s cluster
  - Build pipeline
  - Registry

Standby: us-phoenix-1
  - GitOps repo synced
  - Object Storage cross-region replication
  - Database cross-region backup
  - Manual failover process documented
```

---

## Implementation Roadmap

### Week 1: Foundation
- [ ] Create OCI Container Registry (OCIR)
- [ ] Create ARM64 VM (4 OCPU, 24GB RAM)
- [ ] Install coordinator + runner on ARM64 VM
- [ ] Update runner to push to OCIR
- [ ] Update k3s image pull secrets
- [ ] Decommission 2 AMD64 VMs
- [ ] Test end-to-end build + deploy with OCIR

### Week 2: Persistence
- [ ] Create Autonomous JSON Database
- [ ] Migrate coordinator from SQLite to JSON DB
- [ ] Set up OCI Vault, migrate all secrets
- [ ] Create Object Storage bucket for build cache
- [ ] Configure Docker Buildx remote cache
- [ ] Set up OCI Monitoring + Alarms

### Week 3: Reliability
- [ ] Request OCI Load Balancer
- [ ] Configure custom domain
- [ ] Attach Block Volume to k3s
- [ ] Enable VCN Flow Logs
- [ ] Document disaster recovery process

### Week 4: Polish
- [ ] API Gateway (optional)
- [ ] OCI Functions for webhooks (optional)
- [ ] Performance testing
- [ ] Security audit
- [ ] Documentation update

---

## Cost Analysis (Always Free Only)

| Service | Free Tier Limit | Used By | Status |
|---------|----------------|---------|--------|
| Compute AMD | 2 VMs, 1/8 OCPU, 1GB | ~~Coordinator, Runner~~ → **Eliminated** | ✅ Reclaimed |
| Compute ARM | 4 VMs, 24GB, 3K OCPU hrs | k3s control + builder | ✅ Using 2 |
| Block Volume | 200GB, 2 volumes | k3s data + backups | ✅ Using 100GB |
| Object Storage | 20GB standard | Build cache, artifacts | ✅ Using ~5GB |
| Container Registry | 10GB | All app images | ✅ Replacing GHCR |
| Autonomous DB | 2 instances, 20GB | Build history | ✅ Week 2 |
| Vault | 20 key versions, 150 secrets | All secrets | ✅ Week 2 |
| Load Balancer | 1 instance, 10 Mbps | Ingress | ✅ Week 3 |
| Monitoring | 500M ingestion points | Infrastructure | ✅ Week 2 |
| VCN | 2 networks | Private network | ✅ In use |
| Flow Logs | 10GB/month | Security audit | ✅ Week 3 |
| Bastion | 5 instances | Emergency SSH | ✅ Available |

**Total monthly cost**: $0 (Always Free only)

---

## Decision Log

### 1. Why OCIR over GHCR?
- **GHCR**: 500MB limit, private by default, separate auth per repo, rate limits
- **OCIR**: 10GB limit, tenancy-scoped auth, same region as cluster, no visibility toggle, integrated IAM
- **Verdict**: OCIR wins on every dimension

### 2. Why ARM64 builder instead of AMD64 + QEMU?
- **AMD64 + QEMU**: 24 min build time, high CPU overhead, potential emulation bugs
- **ARM64 native**: 2-3 min build time, no emulation, same architecture as k3s nodes
- **Verdict**: ARM64 native is 10x faster and more reliable

### 3. Why JSON DB over SQLite?
- **SQLite**: Fast, simple, but local-only, no HA, hard to query
- **JSON DB**: Managed, persistent, queryable, scalable, free tier sufficient
- **Verdict**: JSON DB for production; SQLite acceptable for dev only

### 4. Why keep VictoriaMetrics + add OCI Monitoring?
- **VictoriaMetrics**: Already deployed, works well, PromQL-native, detailed pod metrics
- **OCI Monitoring**: Infrastructure-level metrics, alarms, integrated with OCI ecosystem
- **Verdict**: Hybrid — VictoriaMetrics for app metrics, OCI Monitoring for infra + alarms

### 5. Why OCI Vault over K8s Secrets?
- **K8s Secrets**: Base64 encoded (not encrypted by default), etcd storage, manual rotation
- **OCI Vault**: HSM-backed, automatic rotation, IAM-controlled, audit logging
- **Verdict**: OCI Vault for all secrets; K8s Secrets only as temporary injection mechanism

---

## Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| ARM64 VM creation fails (capacity) | High | Keep 1 AMD64 VM as fallback; provision during off-peak hours |
| OCIR auth token expires | High | Set calendar reminder; use Vault automatic rotation |
| JSON DB connection latency | Medium | Connection pooling; local SQLite fallback for active builds |
| Load Balancer misconfiguration | High | Test in staging; document rollback procedure |
| Data migration failure | High | Backup SQLite before migration; test restore process |

---

## Next Steps

1. **Approve this plan** (or suggest changes)
2. **Start Week 1**: I can create the ARM64 VM, set up OCIR, migrate the build pipeline
3. **Parallel work**: You create OCIR repo and Vault secrets (requires OCI console access)
4. **Deploy**: Update k3s deployment manifests for new image registry

**Estimated time to full implementation**: 2-3 weeks (part-time)
**Expected build time improvement**: 24 min → 2-3 min (10x)
**Expected reliability improvement**: Eliminates GHCR visibility issues, secret sprawl, and single-VM failure modes

---

*Document version: 1.0*
*Created: 2026-05-30*
*Author: TinyCloud AI Architect*

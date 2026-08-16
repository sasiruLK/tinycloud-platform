# TinyCloud Infrastructure Runbook

Current state of the lab in `us-ashburn-1`, verified `2026-08-16` via authenticated OCI CLI and
`kubectl` through the OCI Bastion. This is the single description of what exists — there is no
pending rebuild; the cluster is healthy and serving. Everything lives in the **root compartment**.

| Item | Value |
|------|-------|
| Region | `us-ashburn-1` |
| Tenancy OCID | `ocid1.tenancy.oc1..aaaaaaaa7xgc5ijlnvzktzftj6ho6jpzymmiira5vhug65pcvtcdy26m3ebq` |
| Object Storage namespace | `idzghas4xwzv` |
| VCN subnet | `10.0.0.0/24` |
| k3s API endpoint | `10.0.0.95:6443` |

## VM Topology

All four instances are `RUNNING`.

| Name | Shape | Arch | RAM | Public IP | Private IP | Role |
|------|-------|------|-----|-----------|------------|------|
| k3s-control | VM.Standard.A1.Flex | ARM64 | 6 GB | 150.136.8.120 | 10.0.0.95 | k3s control plane, Bastion plugin target |
| k3s-worker-1 | VM.Standard.A1.Flex | ARM64 | 6 GB | 132.145.146.113 | 10.0.0.73 | app workloads |
| amd-utility-1 | VM.Standard.E2.1.Micro | AMD64 | 1 GB | — | 10.0.0.122 | lightweight tooling, outside the critical path |
| amd-utility-2 | VM.Standard.E2.1.Micro | AMD64 | 1 GB | — | 10.0.0.55 | spare, normally idle |

No public IP was recorded for either AMD micro in the `2026-08-15` verification. As of `2026-07-03`
the Ampere A1 tenancy limit was `2 OCPUs / 12 GB` regionally — hence two ARM nodes, not four. Not
re-verified since.

## Cluster

| Item | Value |
|------|-------|
| k3s | `v1.36.2+k3s1` |
| Nodes | `k3s-control` (control-plane), `k3s-worker-1` (worker) — both `Ready`, ~42 days uptime |
| OS | Ubuntu 22.04.5 |
| Runtime | containerd `2.3.2-k3s2` |
| Ingress | Traefik `3.7.4`, 6 IngressRoutes serving |
| TLS | cert-manager `v1.15.5`; certificate `tinycloud-platform-tls` is `Ready` |

### GitOps

| Component | Version | Note |
|-----------|---------|------|
| Argo CD | `v3.3.14` in `argocd` | Upgraded from `v2.13.3` on 2026-08-16 |
| argocd-image-updater | `v1.2.2` in `argocd` | Adopted into `argocd/image-updater.yaml` |

Both are now declared in `gitops-lab/argocd/` and reconciled by the `root` app-of-apps, so a rebuild
restores them. That was not true before 2026-08-15, when each existed only as a live object.

ApplicationSet `user-apps` exists. Nine Argo Applications, all `Synced/Healthy`:
`argocd-image-updater`, `blog`, `cert-manager`, `counter-demo`, `external-secrets`, `root`,
`tinycloud-api`, `tinycloud-platform`, `tinycloud-ui`.

Workloads in `tinycloud`: `blog`, `build-coordinator`, `nginx-proxy`, `oauth2-proxy`,
`tinycloud-api`, `tinycloud-ui`.

## Registry — GHCR only

Every workload runs from GHCR (`ghcr.io/sasirulk/...`).

### Cluster Pull Secrets

| Namespace | Required secret |
|-----------|-----------------|
| argocd | `ghcr-creds` |
| tinycloud | `ghcr-creds` |
| each app namespace | `ghcr-creds` |

`ghcr-creds` is present in `argocd` and `tinycloud`. The manifest generator emits it throughout —
deployment pull secret, PreSync reader ClusterRole/Binding, secret sync Job, ImageUpdater pull secret.

### Every secret the cluster needs

| Secret | Namespace | Contents | In Vault | Read from Vault |
|--------|-----------|----------|----------|-----------------|
| `cloudflare-api-token` | `cert-manager` | `api-token` | yes | yes — `argocd/oci-vault-store.yaml` |
| `oauth2-github` | `tinycloud` | `client-id`, `client-secret`, `cookie-secret` | yes | not yet |
| `github-pat` | `tinycloud` | `token`, `username` | yes | not yet |
| `build-coordinator-token` | `tinycloud` | `token` | yes | not yet |
| `ghcr-creds` | `argocd`, `tinycloud`, each app ns | `.dockerconfigjson` | yes | not yet |

Until 2026-08-16 only the first row existed anywhere but the live cluster. The other four were
created by hand and backed up nowhere — a rebuild would have restored every manifest and no
credential, and the failure is not obvious: `oauth2-github` is what console login depends on, so
losing it locks the console out entirely with no way back in.

All five are now in the `tinycloud-secrets` vault, each stored as a base64 JSON map of
`{key: value}` — the shape External Secrets' `dataFrom.extract` expects, so wiring them back needs
no reshaping. The `oauth2-github` copy was verified by round-trip against the live Secret.

**Backed up is not the same as reconciled.** Four of the five are still applied by hand; the Vault
copy is a recovery path, not the source of truth, and it will drift silently the next time one is
rotated in the cluster. Finishing the job means an `ExternalSecret` per row, at which point the
cluster copy becomes derived state. Rotate in Vault first until then.

Nothing was lost when `deploy/` was deleted on 2026-08-16 — its `secret-template.yaml` and
`build-coordinator-secret-template.yaml` were placeholders with fake values, and this table records
what they documented.

### OCIR is impossible on this tenancy

An **authenticated** `oci artifacts container repository list` against this tenancy returns:

```
HTTP 403  code: FREE_TIER_NOT_SUPPORTED
```

This is a tenancy-level block on the artifacts service — **not** an auth, IAM, policy or
`docker login` problem, and no credential fixing changes it. OCIR can never be used here. It has been
purged from the Go code and the manifests; do not reintroduce it.

## OCI Services

| Service | State |
|---------|-------|
| Bastion `tinycloud-lab-bastion` | `ACTIVE`. See [access-runbook.md](./access-runbook.md) |
| Notifications topic `tinycloud-alerts` | `ACTIVE`, with 6 alarms publishing to it |
| Vault `tinycloud-secrets` | `ACTIVE`. Holds `cloudflare-api-token`, read by External Secrets |
| Object Storage | namespace `idzghas4xwzv` |

Six alarms are enabled and wired to the topic: `site-unreachable`, `ingress-backend-unhealthy`,
`instance-unhealthy`, `node-memory-high` (all CRITICAL), `tls-expiring-soon` and `node-cpu-high`
(WARNING).

### OCI Logging

Log group `tinycloud`, custom log `platform-containers`. The build coordinator ships every runner
log line and every build lifecycle transition here as JSON, authenticating as the instance through
the `tinycloud-nodes` dynamic group and the `tinycloud-logging-write` policy. Verified end to end on
2026-08-16.

This exists because build history is otherwise a single SQLite file on a `local-path` volume pinned
to `k3s-worker-1`, and local-path does not survive the node. Search it with:

```
search "<tenancy>/<loggroup>/<log>" | sort by datetime desc
```

Entries carry `type` (`build.log` or `build.status`), `jobId`, `stream`, `message`, and for status
transitions `image`, `tag` and `error`. Shipping is best-effort by design: a full buffer drops
entries rather than blocking the coordinator, since this is the secondary copy.

One entry in job `0f187e4c` is a `[verification]` probe line sent to prove delivery, not build
output.

## Design Rules

- Steady state is **2 ARM nodes**: one control plane, one worker
- **GHCR is the only registry**
- Admin SSH goes through **OCI Bastion**, not a dedicated jump host
- No self-hosted monitoring stack on ARM
- No dedicated ARM build VM
- No build jobs on the AMD micros
- Public network exposure is limited to ingress plus the explicitly required admin paths

## Where User-App Builds Run — Decided

**GitHub Actions.** Settled 2026-08-15; the section that stood here recorded it as unresolved.

`cmd/build-coordinator` needs no Docker host of its own. It keeps the queue, the job lifecycle and
the logs, and dispatches the actual build to a workflow via `repository_dispatch`. The runner
reports back on `/v1/runner/jobs/:id/logs` and `/status`, so build output still lands in the
platform's own database and console — the coordinator was not hollowed out, only its executor moved.

Why this and not the alternatives the old section listed:

- A dedicated ARM build VM is impossible: the Ampere allocation is `2 OCPU / 12 GB` and both ARM
  nodes are fully consumed by k3s.
- Builds on the AMD micros are forbidden by the design rules, and 1 GB of RAM would not survive a
  container build regardless.
- In-cluster BuildKit/Kaniko on `k3s-worker-1` would contend with the workloads it is building for,
  on the one node that also holds the coordinator's database.

`cmd/build-runner` remains in the tree but is not deployed anywhere.

The coordinator itself runs as a pod in `tinycloud` as of 2026-08-16, deployed by Argo CD from
`gitops-lab/apps/tinycloud-platform/build-coordinator.yaml`. It previously ran as a systemd unit on
`k3s-worker-1`, updated by scp'ing a binary; its SQLite database now lives on a `local-path`
PersistentVolumeClaim pinned to that node.

## Removed Tooling

Deleted on 2026-08-15 because OCIR cannot be used on this tenancy:

- `scripts/deploy/setup-ocir.sh`
- `scripts/verify-ocir-argocd.sh`
- `docs/deploy/ocir-setup.md`, `docs/deploy/github-actions-ocir.yml`

`scripts/bootstrap-gitops.sh` was fixed in the same pass: its `APPLY_PLATFORM_APPS=1`
path now requires `ghcr-creds` rather than `ocir-creds`.

Deleted on 2026-08-16, when the build coordinator became a pod. They provisioned and
updated a build VM by hand — cross-compiling a binary, scp'ing it to k3s-worker-1 and
restarting a systemd unit — and `bootstrap-build-vm.sh` still configured OCIR:

- `scripts/deploy/bootstrap-build-vm.sh`
- `scripts/deploy/push-build-vm.sh`
- `scripts/deploy/build-binaries.sh`
- `deploy/` — an unreferenced copy of the API manifests pinned to `:latest`, still
  containing the `tinycloud-api-logs` Role that was replaced by `tinycloud-api-workloads`

The coordinator now deploys the same way as everything else: a push builds an image, Image
Updater advances the tag in gitops-lab, Argo CD syncs it.

## Validation Checklist

- `k3s-control` and `k3s-worker-1` are `Ready` and schedulable
- All Argo Applications are `Synced/Healthy`
- `tinycloud.sasiru.lk` and one app hostname resolve and serve over TLS
- `ghcr-creds` exists in `argocd`, `tinycloud`, and every app namespace
- `tinycloud-platform-tls` is `Ready` and not near expiry

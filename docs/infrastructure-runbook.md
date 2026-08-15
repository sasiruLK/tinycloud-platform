# TinyCloud Infrastructure Runbook

Current state of the lab in `us-ashburn-1`, verified `2026-08-15` via authenticated OCI CLI and
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
| Ingress | Traefik `3.7.4`, 5 IngressRoutes serving |
| TLS | cert-manager `v1.15.5`; certificate `tinycloud-platform-tls` is `Ready` |

### GitOps

| Component | Version | Note |
|-----------|---------|------|
| Argo CD | `v2.13.3` in `argocd` | **installed out of band** |
| argocd-image-updater | `v1.2.2` in `argocd`, running ~34 days | **installed out of band** |

Neither is installed by anything in `tinycloud-platform` or `gitops-lab`. If the cluster is rebuilt,
both must be reinstalled by hand or added to a repo first.

ApplicationSet `user-apps` exists. Five Argo Applications, all `Synced/Healthy`:
`blog`, `cert-manager`, `tinycloud-api`, `tinycloud-platform`, `tinycloud-ui`.

Workloads in `tinycloud`: `blog`, `nginx-proxy`, `oauth2-proxy`, `tinycloud-api`, `tinycloud-ui`.

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
| Notifications topic `tinycloud-alerts` | `ACTIVE`, but **zero alarms are configured**, so nothing ever publishes to it |
| Vault | **does not exist** — no OCI Vault in this tenancy |
| Object Storage | namespace `idzghas4xwzv` |

Alarms are unbuilt work: the topic is a destination with no source.

## Design Rules

- Steady state is **2 ARM nodes**: one control plane, one worker
- **GHCR is the only registry**
- Admin SSH goes through **OCI Bastion**, not a dedicated jump host
- No self-hosted monitoring stack on ARM
- No dedicated ARM build VM
- No build jobs on the AMD micros
- Public network exposure is limited to ingress plus the explicitly required admin paths

## Open Question — Where Do User-App Builds Run?

**Unresolved. Do not treat any option below as decided.**

`cmd/build-coordinator` and `cmd/build-runner` need a Docker host. The design rules above forbid a
dedicated build VM and forbid builds on the AMD micros, and both ARM VMs are consumed by k3s — so
there is nowhere for the build plane to run.

The platform's own images are unaffected: `.github/workflows/build-api.yaml` and
`build-ui.yaml` build the api and ui images on GitHub Actions and push to GHCR. That works.

Moving **user-app** builds to GitHub Actions is a **candidate, not a decision**:

- For: no compute to maintain, nothing idle against the ARM cap, already proven for api/ui.
- Against: it hollows out the coordinator/runner that are the point of the platform, moves builds off
  infrastructure we control, and needs per-user-repo workflow and token plumbing the coordinator
  design existed to avoid.

Not yet costed: ephemeral in-cluster BuildKit/Kaniko pods on `k3s-worker-1` with a concurrency cap of
1, or reshaping the ARM allocation to free a build host.

## Stale Tooling — Do Not Run

These still assume OCIR or the deleted build VM and will fail or do the wrong thing:

- `scripts/deploy/setup-ocir.sh`
- `scripts/verify-ocir-argocd.sh`
- `scripts/deploy/bootstrap-build-vm.sh`
- `scripts/bootstrap-gitops.sh` — its `APPLY_PLATFORM_APPS=1` path still requires `ocir-creds`
- `docs/deploy/ocir-setup.md`, `docs/deploy/github-actions-ocir.yml`

They are out of scope for this doc pass and are recorded here so nobody runs them.

## Validation Checklist

- `k3s-control` and `k3s-worker-1` are `Ready` and schedulable
- All Argo Applications are `Synced/Healthy`
- `tinycloud.sasiru.lk` and one app hostname resolve and serve over TLS
- `ghcr-creds` exists in `argocd`, `tinycloud`, and every app namespace
- `tinycloud-platform-tls` is `Ready` and not near expiry

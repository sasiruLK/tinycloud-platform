# OCI Free-Tier Rebuild Guide

This guide is the execution target for the lab rebuild.

Before any destructive step, run `./scripts/rebuild-preflight.sh` from `tinycloud-platform`.

## Final Shape

- 2 ARM VMs inside k3s
- 2 AMD micro VMs kept outside the critical path
- OCI Bastion for admin access
- OCI Monitoring/Notifications/Logging instead of a self-hosted monitoring VM
- OCIR as the only active image registry
- no dedicated ARM build VM in the base design

## What Runs Where

### k3s-control

- k3s server
- Argo CD
- cert-manager
- only minimal control-plane-safe addons

### k3s-worker-1

- Traefik ingress
- platform support workloads that must live in-cluster
- sample apps

### amd-utility-1 / amd-utility-2

- lightweight CLI tools only
- temporary experiments only
- not part of the main deployment path

## What To Remove From The Old Design

- GHCR from active manifests and build defaults
- AMD micro VMs as image builders
- a dedicated bastion VM
- a dedicated self-hosted monitoring VM
- mixed deployment truth between ad hoc hosts and GitOps

## First Demo To Achieve

1. Push one prebuilt multi-arch or ARM image to OCIR
2. Update GitOps manifests
3. Let Argo CD sync automatically
4. Reach the app via `https://{app}.sasiru.lk/`

## Execution Entry Points

- Preflight:
  - `./scripts/rebuild-preflight.sh`
- k3s cluster bootstrap:
  - `./scripts/bootstrap-k3s-cluster.sh`
- Argo CD + cert-manager bootstrap:
  - `CLOUDFLARE_API_TOKEN=... APPLY_PLATFORM_APPS=0 ./scripts/bootstrap-gitops.sh`
- OCIR repo, token, and `ocir-creds`:
  - `SKIP_BUILD_VM_LOGIN=1 ./scripts/deploy/setup-ocir.sh`
- Base platform apps:
  - `CLOUDFLARE_API_TOKEN=... ./scripts/bootstrap-gitops.sh`

## Acceptance Criteria

- New app deploy flow does not depend on GHCR
- Cluster does not depend on AMD micro VMs to stay healthy
- Admin access works through OCI Bastion
- One worker loss does not destroy the GitOps demonstration
- Steady-state ARM usage stays within `2 OCPUs / 12 GB`

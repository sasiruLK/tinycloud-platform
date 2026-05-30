# TinyCloud Build Infrastructure

Phase 1 moves the build pipeline to a **native ARM64 build-vm** (repurposed `monitoring-vm`, `10.0.0.107`) and pushes images to **OCIR**.

## Topology

| Host | Private IP | Role |
|------|------------|------|
| build-vm (was monitoring-vm) | 10.0.0.107 | Coordinator + runner (native ARM64, Docker Buildx) |
| k3s cluster | — | Pulls from OCIR via `ocir-creds` |
| 1GB AMD VMs | 10.0.0.122 / 10.0.0.55 | Legacy QEMU builders — decommission after cutover |

## Flow

```text
TinyCloud API -> Build Coordinator (build-vm) -> Build Runner (same host) -> OCIR -> gitops-lab -> Argo CD
```

## OCI Vault Secrets

Render into `/etc/tinycloud/*.env` at boot (OCI CLI + instance principals):

- `BUILD_COORDINATOR_TOKEN` — shared secret between API, coordinator, and runner
- `GITHUB_TOKEN` — PAT for GitHub (API repos, GitOps commits, private clones)
- `OCIR_AUTH_TOKEN` + `OCIR_USERNAME` — `docker login iad.ocir.io`

See [ocir-setup.md](./deploy/ocir-setup.md) and [infrastructure-runbook.md](./infrastructure-runbook.md).

## Runner Configuration

```bash
IMAGE_PREFIX=iad.ocir.io/idzghas4xwzv/tinycloud
BUILD_CACHE_REF=iad.ocir.io/idzghas4xwzv/tinycloud/cache:buildkit
BUILD_COORDINATOR_URL=http://127.0.0.1:8090   # on build-vm when colocated
```

On ARM64, the runner skips `--platform linux/arm64` (native build). Set `BUILD_PLATFORM=linux/arm64` only for QEMU cross-build on AMD.

## Bootstrap

```bash
# On build-vm (150.136.96.152 / 10.0.0.107)
sudo ./scripts/deploy/bootstrap-build-vm.sh
```

## Runner Prerequisites

```bash
docker login iad.ocir.io -u 'idzghas4xwzv/YOUR_OCI_USERNAME'
docker buildx create --use --name tinycloud 2>/dev/null || docker buildx use tinycloud
```

Images use immutable commit-SHA tags: `iad.ocir.io/idzghas4xwzv/tinycloud/{app}:{sha}`.

BuildKit registry cache avoids Object Storage API limits (50k req/month).

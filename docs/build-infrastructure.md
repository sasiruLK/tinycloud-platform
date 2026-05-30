# TinyCloud Build Infrastructure

Phase 1 uses two standalone AMD VMs.

- AMD VM #2 runs `tinycloud-build-coordinator` on the private VCN.
- AMD VM #1 runs `tinycloud-build-runner` with Docker Buildx.
- TinyCloud API talks to the coordinator through `BUILD_COORDINATOR_URL`.
- Secrets are loaded from OCI Vault into the systemd environment files before service start.

## Flow

```text
TinyCloud API -> Build Coordinator -> Build Runner -> GHCR -> gitops-lab -> Argo CD
```

## OCI Vault Secrets

Store these as Vault secrets and render them into `/etc/tinycloud/*.env` using instance principals:

- `BUILD_COORDINATOR_TOKEN` — shared secret between API, Coordinator, and Runner
- `GITHUB_TOKEN` — single PAT for all GitHub operations:
  - API: listing user repositories
  - Coordinator: writing manifests to `gitops-lab`
  - Runner: cloning private source repositories
- GHCR credentials for `docker login ghcr.io`

The services intentionally read secrets from environment variables so the VM bootstrap can use the OCI CLI without linking the platform binaries to the OCI SDK.

## Runner Prerequisites

```bash
docker login ghcr.io
docker buildx create --use --name tinycloud
```

The runner builds `linux/arm64` images and pushes immutable tags based on the source commit SHA.

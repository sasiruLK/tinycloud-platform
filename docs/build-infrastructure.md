# TinyCloud Build Infrastructure — Historical

> **This describes a system that no longer exists.** Kept as a record of the Phase 1 build design.
> For current state see [infrastructure-runbook.md](./infrastructure-runbook.md).

Two things in the design below are dead as of `2026-08-15`:

- **`build-vm` at `10.0.0.107` is gone.** There is no dedicated build host. The lab is four VMs:
  two ARM nodes in k3s and two AMD micros.
- **OCIR is impossible on this tenancy.** An authenticated
  `oci artifacts container repository list` returns HTTP 403 `FREE_TIER_NOT_SUPPORTED`. Everything
  runs from GHCR (`ghcr.io/sasirulk/...`) with the `ghcr-creds` pull secret.

There is also no OCI Vault in this tenancy, so the Vault-rendered `/etc/tinycloud/*.env` boot flow
below was never realised.

## What Actually Builds Today

`.github/workflows/build-api.yaml` and `.github/workflows/build-ui.yaml` build the platform api and
ui images on GitHub Actions and push them to GHCR. That path works.

**Settled 2026-08-15: user-app builds run in GitHub Actions.** The coordinator needs no Docker host
of its own — it keeps the queue, lifecycle and logs and dispatches the build to a workflow via
`repository_dispatch`, and the workflow reports back on `/v1/runner/*`. `cmd/build-runner`, the
polling executor that did need a Docker host, has been deleted; only `cmd/api` and
`cmd/build-coordinator` remain. The coordinator itself runs as a pod in `tinycloud` as of 2026-08-16.

The coordinator still exposes `/v1/runner/poll` for a polling runner. Nothing calls it.

## Historical Design (Phase 1, superseded)

| Host | Private IP | Role |
|------|------------|------|
| build-vm | 10.0.0.107 | Coordinator + runner (native ARM64, Docker Buildx) — **terminated** |
| k3s cluster | — | Pulled from OCIR via `ocir-creds` — **OCIR unavailable** |
| amd-utility-1 / amd-utility-2 | 10.0.0.122 / 10.0.0.55 | Utility hosts outside the critical path |

```text
TinyCloud API -> Build Coordinator (build-vm) -> Build Runner (same host) -> OCIR -> gitops-lab -> Argo CD
```

Intended secrets, rendered from OCI Vault via instance principals: `BUILD_COORDINATOR_TOKEN`,
`GITHUB_TOKEN`, and `OCIR_AUTH_TOKEN` / `OCIR_USERNAME`. The runner skipped `--platform linux/arm64`
on ARM64 and set `BUILD_PLATFORM` only for QEMU cross-builds on AMD. Images used immutable commit-SHA
tags; a BuildKit registry cache avoided the Object Storage 50k requests/month API cap.

`scripts/deploy/setup-ocir.sh`, `docs/deploy/ocir-setup.md` and
`docs/deploy/github-actions-ocir.yml` were deleted on 2026-08-15.
`scripts/deploy/bootstrap-build-vm.sh`, `push-build-vm.sh` and `build-binaries.sh` were
deleted on 2026-08-16. The build-plane question they were waiting on is settled: builds run
in GitHub Actions, and the coordinator that queues them runs as a pod in `tinycloud`, deployed
by Argo CD from `gitops-lab/apps/tinycloud-platform/build-coordinator.yaml`.

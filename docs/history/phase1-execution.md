# Phase 0 + Phase 1 Execution Notes

> Historical execution log, and much of it is now false: `build-vm` (10.0.0.107) no longer exists and
> OCIR returns HTTP 403 `FREE_TIER_NOT_SUPPORTED` on this tenancy, so the OCIR steps below are void
> and the lab runs entirely on GHCR. `scripts/deploy/setup-ocir.sh` has been deleted.
> For current state use [infrastructure-runbook.md](../infrastructure-runbook.md).

Executed 2026-05-30.

## Phase 0 — Done

- [x] VM inventory with private IPs — [infrastructure-runbook.md](../infrastructure-runbook.md)
- [x] `./scripts/phase0-inventory.sh` added (run with OCI CLI for storage/GHCR audit)
- [x] OCI namespace confirmed: `idzghas4xwzv`

## Phase 1 — Done / In progress

### Completed

- [x] OCIR repository `tinycloud` created in tenancy root compartment
- [x] Runner code: OCIR `IMAGE_PREFIX`, BuildKit registry cache, native ARM64 (no `--platform` on arm64)
- [x] Manifest generator: `ocir-creds` for new app deployments
- [x] build-vm bootstrapped on `build-vm` (150.136.96.152 / 10.0.0.107)
- [x] Coordinator + runner running on build-vm (native ARM64)
- [x] AMD runner on `amd-utility-1` stopped
- [x] `tinycloud-api` `BUILD_COORDINATOR_URL` → `http://10.0.0.107:8090` (gitops + live kubectl patch)
- [x] `ocir-creds` secret in `argocd` and `tinycloud` namespaces

### Manual follow-up

- [ ] **OCIR docker login on build-vm** — run locally (OCI CLI authenticated):
  ```bash
  # setup-ocir.sh deleted: OCIR is unavailable on this tenancy
  ```
- [ ] Copy coordinator SQLite from `amd-utility-2` (10.0.0.55) only if build history is worth keeping
- [ ] Add `ocir-creds` to existing app namespaces (e.g. `htmx-go-counter`) before switching image refs from GHCR
- [ ] Rebuild/redeploy existing apps to OCIR on next build
- [ ] Stop coordinator on `amd-utility-2` after verifying builds, or rebuild that host cleanly instead
- [ ] Phase 2: offload monitoring stack from build-vm (VictoriaMetrics/Grafana/Loki still on Docker)

## Verify

```bash
# From k3s-control or any VCN host
curl http://10.0.0.107:8090/health

# On build-vm
systemctl status tinycloud-build-coordinator tinycloud-build-runner
sudo -u tinycloud docker buildx ls   # should show linux/arm64 native
```

## Rollback

**This rollback is no longer executable, and is kept only as a record of what was done at the time.**
`tinycloud-build-runner` no longer exists on `amd-utility-1` or anywhere else, `cmd/build-runner` has
been deleted, and the coordinator moved off `build-vm` into a pod on 2026-08-16. Builds run in GitHub
Actions.

1. ~~Re-enable AMD runner: `sudo systemctl enable --now tinycloud-build-runner` on `amd-utility-1`~~
2. Revert API coordinator URL to `http://10.0.0.55:8090`
3. Set `IMAGE_PREFIX` back to `ghcr.io/sasirulk` in runner env

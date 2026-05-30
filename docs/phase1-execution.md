# Phase 0 + Phase 1 Execution Notes

Executed 2026-05-30.

## Phase 0 — Done

- [x] VM inventory with private IPs — [infrastructure-runbook.md](./infrastructure-runbook.md)
- [x] `./scripts/phase0-inventory.sh` added (run with OCI CLI for storage/GHCR audit)
- [x] OCI namespace confirmed: `idzghas4xwzv`

## Phase 1 — Done / In progress

### Completed

- [x] OCIR repository `tinycloud` created in tenancy root compartment
- [x] Runner code: OCIR `IMAGE_PREFIX`, BuildKit registry cache, native ARM64 (no `--platform` on arm64)
- [x] Manifest generator: `ocir-creds` for new app deployments
- [x] build-vm bootstrapped on `monitoring-vm` (150.136.96.152 / 10.0.0.107)
- [x] Coordinator + runner running on build-vm (native ARM64)
- [x] AMD runner (1GB-vm-1) stopped
- [x] `tinycloud-api` `BUILD_COORDINATOR_URL` → `http://10.0.0.107:8090` (gitops + live kubectl patch)
- [x] `ocir-creds` secret in `argocd` and `tinycloud` namespaces

### Manual follow-up

- [ ] **OCIR docker login on build-vm** — run locally (OCI CLI authenticated):
  ```bash
  cd tinycloud-platform && ./scripts/deploy/setup-ocir.sh
  ```
- [ ] Copy coordinator SQLite from 1GB-vm-2 (10.0.0.55) if build history needed — SSH to that VM is blocked by motd; use OCI Console serial console or fix `/etc/motd` script
- [ ] Add `ocir-creds` to existing app namespaces (e.g. `htmx-go-counter`) before switching image refs from GHCR
- [ ] Rebuild/redeploy existing apps to OCIR on next build
- [ ] Stop coordinator on 1GB-vm-2 after verifying builds
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

1. Re-enable AMD runner: `sudo systemctl enable --now tinycloud-build-runner` on 1GB-vm-1
2. Revert API coordinator URL to `http://10.0.0.55:8090`
3. Set `IMAGE_PREFIX` back to `ghcr.io/sasirulk` in runner env

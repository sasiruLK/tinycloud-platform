# TinyCloud Infrastructure Runbook

Phase 0 inventory for the OCI Always Free deployment. Region: **us-ashburn-1**.

## VM Inventory

| Name | Public IP | Private IP | Arch | RAM | Role | Build services |
|------|-----------|------------|------|-----|------|----------------|
| k3s-control | 150.136.8.120 | 10.0.0.95 | ARM64 | 6 GB | K3s control plane | — |
| k3s-worker-1 | 132.145.146.113 | 10.0.0.73 | ARM64 | 6 GB | App workloads | — |
| k3s-worker-2 | 132.145.154.29 | 10.0.0.229 | ARM64 | 6 GB | App workloads | — |
| monitoring-vm → **build-vm** | 150.136.96.152 | **10.0.0.107** | ARM64 | 6 GB | Monitoring (Phase 2 offload) → native ARM64 builder | Target for Phase 1 |
| 1GB-vm-1 | 129.153.180.28 | 10.0.0.122 | AMD64 | 1 GB | Build runner (QEMU) | `tinycloud-build-runner` active |
| 1GB-vm-2 | 157.151.214.150 | 10.0.0.55 | AMD64 | 1 GB | Build coordinator | `tinycloud-build-coordinator` (SQLite DB) |

Hostname on monitoring-vm: `instance-20260518-2331`.

## VCN

- Subnet: `10.0.0.0/24` (private IPs above)
- k3s API server (control node): `10.0.0.95:6443`
- TinyCloud API → coordinator: currently `http://10.0.0.55:8090` → **Phase 1 target** `http://10.0.0.107:8090`

## OCI Object Storage (known)

| Setting | Value |
|---------|-------|
| Namespace | `idzghas4xwzv` |
| Backup bucket | `tinycloud-backups` |
| Region | `us-ashburn-1` |

Run `./scripts/phase0-inventory.sh` on a machine with OCI CLI configured to refresh usage numbers.

## OCIR (Phase 1)

| Setting | Value |
|---------|-------|
| Registry | `iad.ocir.io` |
| Repository prefix | `idzghas4xwzv/tinycloud` |
| App image example | `iad.ocir.io/idzghas4xwzv/tinycloud/{app}:{commit-sha}` |
| BuildKit cache | `iad.ocir.io/idzghas4xwzv/tinycloud/cache:buildkit` |

Storage shares the **20 GB** Always Free Object Storage budget with backups and artifacts.

## k3s Image Pull Secrets

| Namespace | Secret | Phase 1 action |
|-----------|--------|----------------|
| argocd | `ghcr-creds` | Add `ocir-creds` (keep ghcr during migration) |
| tinycloud | `ghcr-creds` | Add `ocir-creds` |
| `{app}` | `ghcr-creds` | New apps use `ocir-creds` from manifest generator |

## SSH Access

```bash
KEY=~/.ssh/ssh-key-2026-05-16.key
ssh -i "$KEY" ubuntu@150.136.96.152   # build-vm (monitoring-vm)
ssh -i "$KEY" ubuntu@150.136.8.120    # k3s-control
```

## Phase 1 Migration Checklist

1. [ ] Run `scripts/phase0-inventory.sh` and record GHCR + Object Storage usage
2. [ ] Create OCIR repos and auth token (see `docs/deploy/ocir-setup.md`)
3. [ ] Bootstrap build-vm: `scripts/deploy/bootstrap-build-vm.sh`
4. [ ] Copy coordinator SQLite from 1GB-vm-2 to build-vm
5. [ ] Update `BUILD_COORDINATOR_URL` in gitops-lab `apps/tinycloud-api/deployment.yaml`
6. [ ] Create `ocir-creds` secret in cluster namespaces
7. [ ] Stop runner on 1GB-vm-1; verify end-to-end build
8. [ ] Decommission or repurpose AMD VMs

# TinyCloud Infrastructure Runbook

Current target state for the OCI Always Free rebuild in `us-ashburn-1`.
As of `2026-07-03`, the tenancy limit for Ampere A1 is `2 OCPUs / 12 GB` regionally, so this runbook targets a 2-node ARM cluster.

## VM Topology

| Name | Public IP | Private IP | Arch | RAM | Role |
|------|-----------|------------|------|-----|------|
| k3s-control | 150.136.8.120 | 10.0.0.95 | ARM64 | 6 GB | k3s control plane |
| k3s-worker-1 | 132.145.146.113 | 10.0.0.73 | ARM64 | 6 GB | app workloads |
| amd-utility-1 | 129.153.180.28 | 10.0.0.122 | AMD64 | 1 GB | optional lightweight tooling |
| amd-utility-2 | 157.151.214.150 | 10.0.0.55 | AMD64 | 1 GB | spare / normally idle |

## Design Rules

- Keep the steady state at **2 ARM nodes** total: one control plane and one worker
- Use **OCI Bastion** for admin SSH instead of dedicating a VM as a jump host
- Use **OCIR only** for image distribution and registry cache
- Do not run a self-hosted monitoring stack on ARM in the base rebuild
- Do not rely on a dedicated ARM build VM in the base rebuild
- Do not run build jobs on the AMD micro instances

## Networking

- VCN subnet: `10.0.0.0/24`
- k3s API endpoint: `10.0.0.95:6443`
- Public access should be limited to ingress traffic and explicitly required SSH/admin paths

## OCIR

| Setting | Value |
|---------|-------|
| Registry | `iad.ocir.io` |
| Namespace | `idzghas4xwzv` |
| Repository prefix | `iad.ocir.io/idzghas4xwzv/tinycloud` |
| BuildKit cache | `iad.ocir.io/idzghas4xwzv/tinycloud/cache:buildkit` |

OCIR shares the Always Free Object Storage budget, so old tags and cache layers must be pruned routinely.

## Access Model

- Preferred admin path: **OCI Bastion**
- Break-glass path: OCI-native console access if Bastion/SSH is unavailable
- Goal after rebuild: avoid relying on direct public SSH to every VM

## Cluster Pull Secrets

| Namespace | Required secret |
|-----------|-----------------|
| argocd | `ocir-creds` |
| tinycloud | `ocir-creds` |
| each app namespace | `ocir-creds` |

## Rebuild Order

1. Preserve only required secrets, DNS settings, and any app data worth keeping
2. Validate OCI Bastion access from the admin machine
3. Rebuild or recover the 2 ARM k3s nodes and 2 AMD VMs cleanly
4. Form the 2-node k3s cluster
5. Install Argo CD and cert-manager
6. Configure OCIR auth and create `ocir-creds`
7. Apply the base GitOps apps
8. Deploy one sample service end to end
9. Add OCI Monitoring, Notifications, and tightly scoped Logging

## Validation Checklist

- `k3s-control` and `k3s-worker-1` are healthy and schedulable
- Argo CD syncs `gitops-lab` without manual fixes
- `tinycloud.sasiru.lk` and one sample app hostname resolve and serve traffic
- OCI Monitoring alarms and Notifications work

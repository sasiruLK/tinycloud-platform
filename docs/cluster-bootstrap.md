# Cluster Bootstrap

> **Partly stale (2026-08-15).** The cluster is up and healthy — nothing here needs running today.
> Every OCIR step below is void: OCIR returns HTTP 403 `FREE_TIER_NOT_SUPPORTED` on this tenancy and
> the lab runs on GHCR with `ghcr-creds`. `scripts/deploy/setup-ocir.sh` and the `ocir-creds` gate in
> `scripts/bootstrap-gitops.sh` have not been updated yet. Note also that Argo CD and
> argocd-image-updater were installed **out of band** and are not reinstalled by these scripts.
> For current state use [infrastructure-runbook.md](./infrastructure-runbook.md).

Run these steps from the admin machine after the ARM and AMD instances have been rebuilt cleanly.
The current Always Free target is `2 OCPUs / 12 GB` on Ampere A1, so the default shape is now `k3s-control + k3s-worker-1` only.

## Order

1. Run `./scripts/rebuild-preflight.sh`
2. Form the k3s cluster with `./scripts/bootstrap-k3s-cluster.sh`
3. Install Argo CD and cert-manager only:
   `APPLY_PLATFORM_APPS=0 CLOUDFLARE_API_TOKEN=... ./scripts/bootstrap-gitops.sh`
4. Create OCIR repo, token, and cluster pull secrets:
   `SKIP_BUILD_VM_LOGIN=1 ./scripts/deploy/setup-ocir.sh`
5. Apply the base platform apps:
   `CLOUDFLARE_API_TOKEN=... ./scripts/bootstrap-gitops.sh`
6. Deploy one sample app through `gitops-lab`

## Cluster Formation

`./scripts/bootstrap-k3s-cluster.sh` installs:

- k3s server on `k3s-control`
- k3s agent on `k3s-worker-1`
- local kubeconfig at `~/.kube/tinycloud-oci.yaml`

Defaults baked into the script:

- k3s version `v1.30.2+k3s1`
- control plane advertises on `10.0.0.95`
- kubeconfig points back to public control-plane IP `150.136.8.120`
- Traefik remains enabled

Override examples:

```bash
KUBECONFIG_OUT=$HOME/.kube/lab.yaml K3S_VERSION=v1.30.3+k3s1 ./scripts/bootstrap-k3s-cluster.sh
```

## GitOps Bootstrap

`./scripts/bootstrap-gitops.sh` does two phases:

- always:
  - installs Argo CD into `argocd`
  - applies `gitops-lab/argocd/cert-manager.yaml`
  - waits for cert-manager
  - creates `cloudflare-api-token`
  - applies `gitops-lab/argocd/cluster-issuers.yaml`
- when `APPLY_PLATFORM_APPS=1`:
  - requires `ocir-creds` in `argocd` and `tinycloud`
  - applies `tinycloud-platform`, `tinycloud-api`, `tinycloud-ui`, and `applicationset-user-apps`

`./scripts/deploy/setup-ocir.sh` now supports a cluster-only path:

```bash
SKIP_BUILD_VM_LOGIN=1 ./scripts/deploy/setup-ocir.sh
```

Required input:

```bash
export CLOUDFLARE_API_TOKEN='...'
```

or:

```bash
export CLOUDFLARE_API_TOKEN_FILE=$HOME/.config/tinycloud/cloudflare-token
```

## Validation

After cluster formation:

```bash
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml get nodes -o wide
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n kube-system get pods
```

After GitOps bootstrap:

```bash
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n argocd get applications
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml get clusterissuers
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n tinycloud get pods
```

## Failure Rules

- If `bootstrap-gitops.sh` stops on missing `ocir-creds`, run `./scripts/deploy/setup-ocir.sh` and rerun it.
- Use `SKIP_BUILD_VM_LOGIN=1` whenever the lab is running without a dedicated ARM build host.
- If guest OS hostnames still look stale after direct SSH, trust the OCI display names and IPs until the rebuild is complete.

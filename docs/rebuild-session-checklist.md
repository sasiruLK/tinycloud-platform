# Rebuild Session Checklist

Resume from this file when the OCI lab rebuild starts.

## Current Known State

- Current live nodes:
  - `k3s-control` `150.136.8.120`
  - `k3s-worker-1` `132.145.146.113`
  - `amd-utility-1` `129.153.180.28`
  - `amd-utility-2` `157.151.214.150`
- OCI Bastion exists:
  - `ocid1.bastion.oc1.iad.amaaaaaaul44qqiax2v6kabqowojtterbpevcp2yviv7ipf6daot3qhnt42a`
- `k3s-worker-2` has been terminated to stay inside the Ampere Always Free limit.

## Admin Defaults

```bash
export SSH_KEY="$HOME/.ssh/id_ed25519"
export OCI_CMD='docker run --rm -i -v "$HOME/.oci:/oracle/.oci" oci'
```

## Preflight

Run this before any destructive rebuild step:

```bash
./scripts/rebuild-preflight.sh
```

Latest known result:

- OCI auth: OK
- Bastion: ACTIVE
- Admin access should use OCI Bastion

Note: some guests still report old OS hostnames over SSH. Treat OCI instance display names as the source of truth until the hosts are rebuilt.

## Target Shape

- 2 ARM VMs in k3s:
  - `k3s-control`
  - `k3s-worker-1`
- 2 AMD micros outside the critical path:
  - `amd-utility-1`
  - `amd-utility-2`

## Next Execution Order

1. Restore admin access to `k3s-control` or place the kubeconfig at `~/.kube/tinycloud-oci.yaml`.
2. Validate current cluster state:
   ```bash
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml get nodes -o wide
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n argocd get applications
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n tinycloud get deploy,pod,svc
   ```
3. If Argo CD or cert-manager is missing, run:
   ```bash
   CLOUDFLARE_API_TOKEN=... APPLY_PLATFORM_APPS=0 ./scripts/bootstrap-gitops.sh
   ```
4. Configure OCIR and create `ocir-creds`:
   ```bash
   SKIP_BUILD_VM_LOGIN=1 ./scripts/deploy/setup-ocir.sh
   ```
5. Apply base GitOps apps:
   ```bash
   CLOUDFLARE_API_TOKEN=... ./scripts/bootstrap-gitops.sh
   ```
6. Commit and push the `gitops-lab` OCIR manifest changes.
7. Prove the base platform deploy:
   ```bash
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n argocd get application tinycloud-platform tinycloud-api tinycloud-ui
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n tinycloud rollout status deploy/tinycloud-api
   kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n tinycloud rollout status deploy/tinycloud-ui
   curl -I https://tinycloud.sasiru.lk/
   ```
8. Prove one user app deploy from OCIR through GitOps:
   - build and push an ARM64 image to `iad.ocir.io/idzghas4xwzv/tinycloud/<app>:<commit-sha>`
   - create or update `gitops-lab/apps/<app>`
   - wait for the ApplicationSet-created Argo CD app to become `Synced/Healthy`
   - verify `https://<app>.sasiru.lk/`

## Latest Session Evidence

Updated on 2026-07-03:

- `gitops-lab` platform API/UI manifests now render OCIR images with immutable tag `f67788cea249eb0b647e80af115bb89a96b7d32e`.
- `gitops-lab` platform API/UI manifests now use `ocir-creds`; no active platform API/UI render path points at GHCR or `latest`.
- GitOps OCIR switch was committed and pushed to `origin/main`:
  - `03ab673 feat: switch gitops manifests to ocir`
- Local render checks passed:
  ```bash
  kubectl kustomize apps/tinycloud-api
  kubectl kustomize apps/tinycloud-ui
  kubectl kustomize apps/tinycloud-platform
  ```
- Backend tests passed:
  ```bash
  GOCACHE=/tmp/tinycloud-go-cache GOPATH=/tmp/tinycloud-go GOTMPDIR=/tmp go test ./...
  ```
- UI production build passed:
  ```bash
  npm ci
  npm run build
  ```
- UI lint passed:
  ```bash
  npm run lint
  ```
- Live cluster verification is not complete:
  - `~/.kube/tinycloud-oci.yaml` is missing locally.
  - SSH to `ubuntu@150.136.8.120` with `$HOME/.ssh/id_ed25519` returns `Permission denied (publickey)`.
  - SSH to `ubuntu@132.145.146.113` previously worked during this session and showed `k3s-agent` active, but later checks with the same key returned `Permission denied (publickey)`.
  - From the earlier successful `k3s-worker-1` check, `10.0.0.95:6443` was reachable and returned Kubernetes `401 Unauthorized`, proving the API server was up but credentials were not available in this session.
  - `tinycloud.sasiru.lk` did not resolve from this environment after the GitOps push.
  - Unauthenticated OCIR `docker manifest inspect` returns `unknown: Free tier account is not supported`; verify image existence after `docker login iad.ocir.io`.

## Fast Verification Commands

```bash
ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no -i "$SSH_KEY" ubuntu@150.136.8.120 'hostname'
ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no -i "$SSH_KEY" ubuntu@132.145.146.113 'hostname'
ssh -F /dev/null -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no -i "$SSH_KEY" ubuntu@129.153.180.28 'hostname'
```

## Repo Entry Points

- Infra target state:
  - `tinycloud-platform/docs/infrastructure-runbook.md`
- Rebuild design:
  - `tinycloud-platform/docs/rebuild-oci-free-tier.md`
- Cluster bootstrap:
  - `tinycloud-platform/docs/cluster-bootstrap.md`
- k3s bootstrap:
  - `tinycloud-platform/scripts/bootstrap-k3s-cluster.sh`
- GitOps bootstrap:
  - `tinycloud-platform/scripts/bootstrap-gitops.sh`
- Rebuild preflight:
  - `tinycloud-platform/scripts/rebuild-preflight.sh`
- OCIR setup:
  - `tinycloud-platform/scripts/deploy/setup-ocir.sh`
- GitOps source of truth:
  - `gitops-lab/README.md`

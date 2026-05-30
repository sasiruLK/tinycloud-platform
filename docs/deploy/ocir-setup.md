# OCIR Setup (Phase 1)

## 1. Create repositories

In OCI Console → Developer Services → Container Registry (region **us-ashburn-1**):

Create repository path: **`tinycloud`** (full path `idzghas4xwzv/tinycloud/...`).

Images pushed by the runner:

- `iad.ocir.io/idzghas4xwzv/tinycloud/{app-name}:{commit-sha}`
- `iad.ocir.io/idzghas4xwzv/tinycloud/cache:buildkit` (BuildKit layer cache)

## 2. Auth token

1. OCI Console → Profile → User Settings → Auth Tokens → Generate
2. Store in OCI Vault as `ocir-auth-token`
3. Username format: `{tenancy-namespace}/{oci-username}` e.g. `idzghas4xwzv/sasiru@example.com`

## 3. Run setup (local OCI CLI preferred)

With `~/.oci/config` authenticated locally:

```bash
cd tinycloud-platform && ./scripts/deploy/setup-ocir.sh
```

Uses **local OCI CLI** for repo/token operations; still SSHs to k3s-control (kubectl) and build-vm (docker login).

Force remote OCI only if needed:

```bash
OCI_RUN_HOST=ubuntu@150.136.8.120 ./scripts/deploy/setup-ocir.sh
```

## 4. k3s pull secret

On k3s-control:

```bash
kubectl create secret docker-registry ocir-creds \
  --docker-server=iad.ocir.io \
  --docker-username='idzghas4xwzv/YOUR_OCI_USERNAME' \
  --docker-password="$OCIR_AUTH_TOKEN" \
  -n argocd --dry-run=client -o yaml | kubectl apply -f -

for ns in tinycloud argocd; do
  kubectl get secret ocir-creds -n argocd -o yaml | \
    sed "s/namespace: argocd/namespace: $ns/" | \
    kubectl apply -f -
done
```

Copy `ocir-creds` into each app namespace when onboarding (or use a sync script).

## 5. Storage budget

OCIR shares the **20 GB** Always Free Object Storage pool with backups and artifacts. Prune old image tags regularly:

```bash
# List tags in OCIR via Console or oci artifacts commands
```

## 6. Verify

```bash
docker pull iad.ocir.io/idzghas4xwzv/tinycloud/cache:buildkit || true
# After first build:
docker pull iad.ocir.io/idzghas4xwzv/tinycloud/{app}:{sha}
```

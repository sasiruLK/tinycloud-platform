# Access Runbook

How to get into the lab. Verified `2026-08-15`. Replaces the old rebuild session checklist — there is
no rebuild pending, the cluster is healthy, and admin access is working.

For what exists and where, see [infrastructure-runbook.md](./infrastructure-runbook.md).

## Why Direct SSH Fails

The workstation was rebuilt and `~/.ssh/id_ed25519` was regenerated on `2026-08-07`. The **new public
key was never added to the instances**, so their `authorized_keys` still hold the old key.

Direct SSH to any instance therefore returns `Permission denied (publickey)`. This is expected. It is
not a network, security-list or sshd fault, and re-running SSH with different flags will not fix it.

The fix is the Bastion: an OCI **managed-SSH** session injects the public key you hand it at session
creation, so the current key works without ever touching `authorized_keys`.

## Bastion

| Item | Value |
|------|-------|
| Name | `tinycloud-lab-bastion` |
| OCID | `ocid1.bastion.oc1.iad.amaaaaaaul44qqiax2v6kabqowojtterbpevcp2yviv7ipf6daot3qhnt42a` |
| State | `ACTIVE` |
| Max session TTL | `10800` seconds (3 hours) |
| Client CIDR allow list | `0.0.0.0/0` |
| Plugin verified `RUNNING` on | `k3s-control` |

Managed-SSH requires the Bastion plugin on the target instance. Confirmed `ENABLED` and used
successfully on both `k3s-control` and `k3s-worker-1` (`2026-08-16`); plugin state on the two AMD
micros is **unverified** — if a session there fails to create,
check the plugin first. The `0.0.0.0/0` client allow list is loose; tightening it is open work.

## Admin Defaults

```bash
export SSH_KEY="$HOME/.ssh/id_ed25519"
export BASTION_ID='ocid1.bastion.oc1.iad.amaaaaaaul44qqiax2v6kabqowojtterbpevcp2yviv7ipf6daot3qhnt42a'
```

## Get a Shell on k3s-control

1. Create a managed-SSH session against the target instance OCID, passing the **current** public key
   and a TTL at or under `10800`:

   ```bash
   oci bastion session create-managed-ssh \
     --bastion-id "$BASTION_ID" \
     --target-resource-id <instance-ocid> \
     --target-os-username ubuntu \
     --ssh-public-key-file "$SSH_KEY.pub" \
     --session-ttl 10800
   ```

2. Wait for the session to reach `ACTIVE`, then read back its SSH command:

   ```bash
   oci bastion session get --session-id <session-ocid>
   ```

   OCI returns the exact command to run. It has the form:

   ```bash
   ssh -i "$SSH_KEY" \
     -o ProxyCommand="ssh -i \"$SSH_KEY\" -W %h:%p -p 22 <session-ocid>@host.bastion.us-ashburn-1.oci.oraclecloud.com" \
     -p 22 ubuntu@10.0.0.95
   ```

Use the command OCI prints, not a hand-built one. Sessions expire — when SSH starts failing again,
check whether the session is still `ACTIVE` before debugging anything else.

### The injected key is removed when the session ends

Worth knowing, because the symptom is confusing: access works, then stops mid-task with
`Permission denied (publickey)` on a host that was answering minutes earlier.

Creating a managed-SSH session makes the Oracle Cloud Agent rewrite the target's
`~/.ssh/authorized_keys`, saving the original as `authorized_keys_backup` (owned by `snap_daemon`).
When the session ends it restores that backup, taking the injected key with it. Nothing is wrong with
the host and nothing needs repairing — create a new session.

The only key that persists across sessions is the instance's original OCI key,
`ssh-key-2026-05-16` (RSA, `SHA256:rSevtbQQ4iimzV+CejkUtrLgOBnhxhdtZ0BGBwKiUHg`). If you still have
that private key, it works without a session; `~/.ssh/id_ed25519` never will on its own.

### Port-forwarding sessions do not inject anything

`create-port-forwarding` only opens a TCP tunnel to the target port. It does not touch
`authorized_keys`, so SSH through one still fails unless the key is already trusted. Use
`create-managed-ssh` for a shell; port-forwarding is for reaching a non-SSH port.

## Permanent Fix (Optional)

Once inside over the Bastion, appending the current public key to `~/.ssh/authorized_keys` on each
host restores direct SSH. Weigh that against the design rule of routing admin access through the
Bastion — the current key-less state is partly the design working as intended.

## kubectl

The cluster API is on `10.0.0.95:6443`, which is private. Reach it through the Bastion, either from a
shell on `k3s-control` or over a port-forward session.

The kubeconfig is `/etc/rancher/k3s/k3s.yaml` on `k3s-control`. Copy it locally, point `server:` at
whichever local endpoint the tunnel exposes, and always pass `--kubeconfig` explicitly:

```bash
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml get nodes -o wide
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n argocd get applications
kubectl --kubeconfig ~/.kube/tinycloud-oci.yaml -n tinycloud get deploy,pod,svc
```

**Never rely on the default kubectl context on the workstation.** It points at an unrelated cluster.
Every command against the lab must carry `--kubeconfig`.

## Checks That Need No Cluster Access

```bash
curl -I https://tinycloud.sasiru.lk/
oci bastion bastion get --bastion-id "$BASTION_ID" --query 'data."lifecycle-state"' --raw-output
```

`./scripts/rebuild-preflight.sh` still does a useful OCI auth plus Bastion state check despite its
name. Ignore its rebuild framing.

## Notes

- Some guests report stale OS hostnames. Treat OCI instance display names as the source of truth.
- Everything is in the **root compartment**; no compartment OCID is needed for these lookups.
- Other docs: `docs/infrastructure-runbook.md` (current state), `docs/architecture-plan.md` and
  `docs/build-infrastructure.md` (history), `gitops-lab/README.md` (GitOps source of truth).

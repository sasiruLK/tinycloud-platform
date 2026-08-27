# Providers

Core holds no cloud credentials. Everything it knows about the infrastructure
an instance runs on, it learns by asking a **provider**: an independent HTTP
service that holds the credentials for one substrate and answers the published
[`/v0` contract](../provider-contract-v0.yaml).

The maintainers' own providers are not special. They are separate processes
answering the same contract over the same HTTP, so anything they can do, a
provider you write can do.

- The contract: [`provider-contract-v0.yaml`](../provider-contract-v0.yaml) at
  the repository root, also served by a running instance at
  `/provider-contract.yaml`.
- Why it is HTTP and not a Go interface: [ADR-0001](adr/0001-providers-as-http-services.md).
- Why metrics are named rather than queried: [ADR-0003](adr/0003-contract-metrics-are-named-not-queried.md).

## What ships today

| Provider kind | Provider | Capabilities |
| --- | --- | --- |
| Infra | `kubernetes` (`cmd/provider-server`) | `instances`, `metrics`, `ingress` |

The Kubernetes provider needs no cloud account: instances are the cluster's
nodes, CPU and memory come from metrics-server when it is installed, and the
ingress address comes from the ingress controller's Service. `alarms` and
`backups` return `501` — a cluster has no native equivalent of either, and
reporting that honestly is what capability discovery is for.

The Oracle Cloud reads are still linked into core, wired only when their
identifiers are configured, until they move out to an Infra provider of their
own. Nothing about a tenancy is compiled in: an unconfigured instance contacts
no account at all.

## Configuring an instance's providers

Core reads its providers from `TINYCLOUD_PROVIDERS` (a JSON list) or
`TINYCLOUD_PROVIDERS_FILE` (a path to the same JSON — a mounted ConfigMap,
typically).

```json
[
  {
    "kind": "infra",
    "name": "kubernetes",
    "baseUrl": "http://tinycloud-provider-kubernetes.tinycloud.svc.cluster.local:9090",
    "tokenFile": "/etc/tinycloud/provider-tokens/kubernetes/token"
  }
]
```

| Field | Meaning |
| --- | --- |
| `kind` | The provider kind. `infra` is the only kind core reads today. |
| `name` | What this provider is called in warnings and logs. |
| `baseUrl` | The root the `/v0` paths hang off. |
| `tokenFile` | Path the bearer token is mounted at. Read per request, so rotation needs no redeploy. |
| `token` | The token inline. For local development only: a token in configuration is a token in `kubectl get`. |

Each capability comes from the first provider that declares it. A capability no
provider serves is a named warning on the dashboard, not an error — a partial
setup still renders, and "not supported" stays distinguishable from "broken".

A malformed provider list stops startup, because it is a mistake. An absent or
unreachable provider does not: it is named in the snapshot's warnings, the last
good snapshot stays on screen flagged stale, and the provider starts answering
by itself when it comes back.

## Migrating an instance already running on Oracle Cloud

Nothing about a tenancy is compiled into the image any more, so an instance
that relied on the old built-in defaults must now name its own account. Add
these to core's Deployment to keep alarms, ingress and backups exactly as they
were:

| Variable | What it names |
| --- | --- |
| `OCI_COMPARTMENT_ID` | The compartment instances, metrics and alarms are read from. |
| `OCI_NLB_ID` | The network load balancer the ingress address comes from. |
| `OCI_OBJECT_STORAGE_NAMESPACE` | The Object Storage namespace. |
| `OCI_BACKUP_BUCKET` | The backup bucket. |

Each is independent: set only the bucket and you get backups, with everything
else reported as unconfigured. Set none — the state of a fresh install — and
core contacts no cloud account at all.

These reads still live inside core and are the last thing there that holds a
cloud credential. They move out to an Infra provider of their own in a separate
piece of work, deliberately sequenced after this one so that the contract was
proven against a second substrate before the first was migrated to it.

## Deploying the Kubernetes provider

[`deploy/provider-kubernetes.yaml`](deploy/provider-kubernetes.yaml) is a
worked example: a Deployment running `provider-server` from the same image as
core, a Service, the RBAC it needs (read nodes, read the ingress Service, read
the metrics API — nothing else), a Secret holding its token, and a default-deny
NetworkPolicy that lets only core reach it.

Two controls, not one. The bearer token is what a provider checks; the network
policy is what stops anything else in the cluster reaching it in the first
place. Mutual TLS between core and a provider is a supported hardening option
and is deliberately not required, so that writing a provider in any language
means setting one header.

### Rotating a provider's token

1. Update the Secret.
2. Wait for the kubelet to project it into both pods (a minute or two).

Both sides read the token per request, so nothing is redeployed and nothing
restarts. Core reads its copy from `tokenFile`; the provider reads its own from
`PROVIDER_TOKEN_FILE`.

## Running it locally

Against any cluster your kubeconfig points at — kind, k3d, minikube, a real one:

```bash
# Terminal 1: the provider, reading the cluster
KUBECONFIG=~/.kube/config PROVIDER_TOKEN=dev-token \
  go run ./cmd/provider-server

# Terminal 2: core, reading the provider
TINYCLOUD_PROVIDERS='[{"kind":"infra","name":"kubernetes","baseUrl":"http://localhost:9090","token":"dev-token"}]' \
KUBECONFIG=~/.kube/config \
  go run ./cmd/api
```

`GET /v1/infra` now serves your cluster's nodes. No cloud account is involved
at any point.

## Writing a provider

1. Generate a client or server from
   [`provider-contract-v0.yaml`](../provider-contract-v0.yaml), in whatever
   language your substrate's SDK is best in.
2. Implement the capabilities you have data for. Return `501` for the rest and
   leave them out of `GET /v0/capabilities` — core will not call an endpoint
   you have not declared, and each one it skips is reported to the operator by
   name.
3. Check the bearer token on every `/v0` path.
4. Prove it works:

   ```bash
   go run ./cmd/conformance -url https://your-provider.example -token "$TOKEN"
   ```

   The conformance suite reports pass, fail or not-implemented per capability
   and exits non-zero only on a genuine contract violation. A provider that
   implements one capability and reports the rest as unimplemented is
   conformant.

The same suite runs in CI against the in-tree providers, so a change to the
contract that breaks an implementation fails our build, not yours.

### On the version prefix

`/v0` makes no stability promise. Per ADR-0001 the contract may change until a
second author has shipped a provider against it; breaking changes are announced
in the changelog, and the prefix moves to `/v1` once a third-party
implementation exists. If you are that second author, say so — the point of the
`v0` is that your implementation gets to shape the contract before it is frozen.

## A word on trust

Installing a third-party provider means running that author's code in your
cluster, holding your substrate's credentials. **Vet a provider like a Helm
chart from a stranger**: read what it does, check what RBAC and secrets it
asks for, and prefer one whose source you can read.

The change is still a net improvement on what came before, where core itself
authenticated to the cloud: the blast radius of a compromised API is now no
cloud credentials at all, and the blast radius of a bad provider is one
substrate.

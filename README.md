# TinyCloud

A self-hostable platform for running applications on small, constrained
Kubernetes clusters. Push to a repository, get a deployed app, on hardware you
could fit inside a free tier.

The reference substrate is an Oracle Cloud Always Free tenancy — four instances,
four ARM cores between them — and that constraint is the point rather than an
apology. Core depends on no cloud vendor: every vendor-specific capability is
reached through a **provider**, an independent HTTP service holding the
credentials for one substrate. Core holds none.

- **Glossary and domain language**: [`CONTEXT.md`](CONTEXT.md)
- **Providers** — the contract, configuring an instance, writing your own:
  [`docs/providers.md`](docs/providers.md)
- **Decisions**: [`docs/adr/`](docs/adr/)

## The provider contract is `/v0`, and carries no stability promise

[`provider-contract-v0.yaml`](provider-contract-v0.yaml) is the published
contract every provider implements. **It may change without notice until a
second author has shipped a provider against it.** Breaking changes are
announced on the issue tracker — this repository has no changelog yet — and the
prefix moves to `/v1` only once a third-party implementation exists.

That is deliberate, and it is [ADR-0001](docs/adr/0001-providers-as-http-services.md):
freezing an interface that has met exactly one implementation is how a project
inherits a bad contract permanently. It has already earned its keep once — the
first outside substrate found the metric capability shaped like its author's
cloud, and it changed before publication rather than after
([ADR-0003](docs/adr/0003-contract-metrics-are-named-not-queried.md)).

If you are writing the second provider, say so on an issue. The point of the
`v0` is that your implementation gets to shape the contract before it is frozen.

Prove yours works with the conformance suite, which is the definition of a
working provider:

```bash
go run ./cmd/conformance -url https://your-provider.example -token "$TOKEN"
```

## Non-goals

TinyCloud runs on any Kubernetes cluster and is designed for small ones. Three
things it deliberately does not do, per
[ADR-0002](docs/adr/0002-constrained-first-scope.md):

- **No HA control plane.** It requires a multi-node control plane that cannot
  run on four cores and cannot be tested here.
- **No multi-region.** It requires the substrate abstraction to model geography.
- **No multi-tenant isolation.** It requires promising security properties about
  a boundary that a shared namespace and network policy layout does not provide.

Horizontal autoscaling and SSO are both supported — HPA is stock Kubernetes, and
oauth2-proxy fronts the API for any OIDC provider.

## Layout

| Path | What it is |
| --- | --- |
| `cmd/api` | Core's API, and the server behind the console |
| `cmd/build-coordinator` | The build coordinator |
| `cmd/provider-server` | Hosts the in-tree providers over the `/v0` contract |
| `cmd/conformance` | The conformance suite |
| `internal/infra` | The infrastructure snapshot: collector, cache, domain types |
| `internal/provider` | The provider client, server, and configuration |
| `ui/` | The console (Vite, TypeScript) |

An instance's deployment lives in its own GitOps repository, not here.

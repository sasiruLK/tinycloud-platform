---
status: accepted
---

# TinyCloud runs anywhere, but is designed for constrained clusters

TinyCloud runs on any Kubernetes cluster, and is designed for small ones: the
reference substrate is four Oracle Cloud Always Free instances totalling 4 ARM
OCPUs, and that constraint shapes the architecture. Portability is a
capability, not a scope: the platform is optimised for constrained clusters and
declines features that only make sense on large ones.

**Non-goals: HA control planes, multi-region, and multi-tenant isolation.**

## Considered options

Positioning as a general self-hostable PaaS was rejected — not because the
platform cannot run on a large cluster, but because competing on that ground
means competing with Coolify, Dokku, and Kamal on features, while the free-tier
constraint is the only thing about this project that is genuinely novel.

## Consequences

- **HPA and SSO are accepted.** HPA is stock Kubernetes and costs nothing to
  allow. SSO already works: oauth2-proxy fronts the API and speaks to any OIDC
  provider.
- **The three refusals each protect the architecture, not just the roadmap.**
  HA requires a multi-node control plane that cannot run on four OCPUs and
  cannot be tested here. Multi-region requires the substrate abstraction to
  model geography. Multi-tenancy requires promising security properties about
  an isolation boundary that the current shared namespace and network policy
  layout does not provide.
- **The non-goals belong in the README, not only in CONTRIBUTING.** Stated
  before a contributor opens a pull request they read as design; stated
  afterwards they read as an excuse.

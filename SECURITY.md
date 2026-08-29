# Security

## Reporting a vulnerability

Use GitHub's [private vulnerability reporting](https://github.com/sasiruLK/tinycloud-platform/security/advisories/new)
on this repository. Please do not open a public issue for a vulnerability.

**There is no response-time commitment.** TinyCloud has one maintainer and this
is not anyone's full-time job. You will get an acknowledgement when it is seen,
which may be days. If that is not good enough for your situation, it is better
to know now than after you have deployed it.

## What is in scope

The code in this repository: core's API, the build coordinator, the provider
server and the in-tree providers, and the console.

An **instance's** deployment — the GitOps repository, the cluster, the secrets
management, the ingress — is the operator's, not this repository's. A finding
about the maintainer's instance is a report about `gitops-lab`, not about
TinyCloud.

## The trust model, stated plainly

**A provider is code you have chosen to run, holding your substrate's
credentials.** Core authenticates to each provider with a bearer token and is
restricted to it by network policy, but nothing in the architecture defends core
against a provider that lies, and nothing defends your substrate against a
provider you installed. **Vet a third-party provider exactly as you would a Helm
chart from a stranger.** See [ADR-0001](docs/adr/0001-providers-as-http-services.md).

**Multi-tenant isolation is a non-goal.** [ADR-0002](docs/adr/0002-constrained-first-scope.md)
declines to promise security properties about a boundary that a shared namespace
and network policy layout does not actually provide. If you are running apps
belonging to people who do not trust each other, TinyCloud is the wrong tool, and
a report that one app can reach another is not a vulnerability — it is this
non-goal working as documented.

**Core still holds Oracle Cloud credentials** when an operator configures them,
because the legacy Oracle read path has not yet moved out to a provider of its
own. Until it does, core's blast radius on an Oracle instance is larger than the
architecture intends. This is tracked as an open issue, and it is the reason the
README's description of core names its own exception.

## Versions

Only the latest release receives fixes. There are no maintained release
branches, and `/v0` of the provider contract carries no stability promise — see
the [`README`](README.md).

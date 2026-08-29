---
status: accepted
---

# Build providers call core; infra providers are called by core

The Infra provider kind works because reads are pull-shaped. Core wants a
snapshot, so core asks: every capability in `/v0` is a `GET`, the provider is an
HTTP service at a URL core holds, and the conformance suite proves a provider by
calling it.

Builds are not shaped like that. "Build this app" is an imperative, and the
thing that executes it is a machine with a container runtime — a GitHub Actions
runner, a build box, a laptop. Requiring core to `POST` to it means requiring
that machine to be reachable from core: a public address or a tunnel, for the
one component most likely to be behind NAT.

**A Build provider is therefore a client of core, not a server core calls.**
Core keeps the queue. The provider asks core for work and reports logs and
status back. `/v0`'s provider-served endpoints stay `GET`-only, and the contract
acquires a second shape rather than a write API.

This is not a new protocol. `internal/build/coordinator` already serves
`/v1/runner/jobs/*`, and the GitHub Actions workflow already speaks it —
today's build plane is this design with one hardcoded executor.

## Considered options

- **Core POSTs jobs to a provider URL**, uniform with the Infra kind. Rejected:
  it needs a reachable endpoint on the executor, and it drags write semantics
  into the contract — idempotency keys, async operation handles, cancellation,
  a failure model much harder than "return `501`". Uniformity is worth a lot,
  but not worth requiring a public address on a build box.
- **Builds are not a provider kind at all**, staying core's job with the
  executor configured. Rejected: it is the reason an instance cannot be run by
  anyone but the maintainer, since the executor is one specific GitHub Actions
  workflow in one specific repository.
- **Both directions**, push where reachable and pull otherwise. Rejected for
  now: two transports for one kind doubles the conformance surface before
  either has an implementation outside this repository.

## Consequences

- **"Provider" no longer implies "HTTP service core calls."** The glossary
  entry is direction-neutral and each kind states which way it faces. No term
  for the distinction is coined yet — per ADR-0001's logic, naming a
  distinction whose second half does not exist is how a project inherits bad
  vocabulary permanently. The term arrives when the Build kind ships.
- **The conformance suite becomes kind-specific.** `-url` cannot reach a
  provider that has no address. Proving a Build provider means the suite
  impersonating core and asserting what the provider polls for, sends, and
  reports. Until that exists, the suite is the definition of a working *Infra*
  provider, and `CONTEXT.md` says so.
- **Authentication runs the other way for this kind.** An Infra provider
  verifies a bearer token core presents. A Build provider presents one to core,
  which is what `BUILD_COORDINATOR_TOKEN` already is.
- **Registry is not a provider kind.** Asked what core would call a registry
  provider *for*, there is no answer that is not "a string it already has"
  — `IMAGE_PREFIX`. The glossary's existing rule for secrets and ingress applies
  unchanged: configuration, not code. Adding the kind later is additive if a
  real operation appears.
- **`/v0` grows rather than becoming `/v1`.** Adding a kind is additive for
  every existing provider, and per ADR-0001 the prefix moves only when a
  third-party implementation exists to be broken.

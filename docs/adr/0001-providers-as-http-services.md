---
status: accepted
---

# Vendor capabilities are HTTP providers, not in-process adapters

TinyCloud must run on substrates other than the Oracle Cloud tenancy it was
built on, and third parties must be able to add substrates without our
involvement. Rather than compile-time Go interfaces, a **provider** is an
independent HTTP service implementing a published OpenAPI contract and deployed
as a Kubernetes Deployment alongside core. Core reaches every vendor capability
— including our own — over that contract.

## Considered options

- **Compile-time Go packages.** Cheapest, and third parties must fork and
  rebuild the binary to add a substrate. Rejected: it makes "anyone can write a
  provider" true only for people willing to maintain a fork, and only for Go.
- **Go `plugin` (`.so`).** Rejected outright: requires a byte-identical
  toolchain and dependency set, and breaks across Go releases.
- **gRPC out-of-process (`go-plugin`).** The Terraform model. Rejected for now
  as heavier for authors than HTTP, with no offsetting benefit at our call
  volume.

## Consequences

- **Core holds no cloud credentials.** Each provider holds only its own
  substrate's credentials. This is a security improvement over the previous
  design, where the API process authenticated to Oracle directly.
- **The new trust boundary is the provider.** Installing a third-party provider
  means running that author's code in your cluster with your cloud credentials.
  Providers must be vetted like a Helm chart from a stranger, and the docs must
  say so.
- **The out-of-process hop is affordable** because `internal/infra/cache.go`
  refreshes on a 60-second TTL and serves the last good snapshot on failure.
  Providers are polled, not called per request.
- **Our own providers ship as in-tree Go, served over the contract** by a thin
  provider server. Core therefore has one code path and we run our own contract
  on every request, so it cannot silently drift from what we implement.
- **The contract is `/v0` with no stability promise** until a second author has
  shipped a provider against it. Freezing interfaces that have met only one
  implementation is how you inherit a bad contract permanently.
- **Providers are wired by explicit configuration**, not label discovery or a
  CRD, while the contract is young. Moving to a `Provider` CRD later is
  additive: it would supply the same configuration from a different source.

# Changelog

Notable changes to TinyCloud. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions are
[semver](https://semver.org/).

**Two versions live here and they move independently.** The version below is the
*release version* — this repository. `/v0` is the *contract version* — the path
prefix providers are served under, which per
[ADR-0001](docs/adr/0001-providers-as-http-services.md) moves to `/v1` only once
a third-party provider exists. A release does not imply a contract change, and
most releases will not be one.

## Unreleased

### Announced, not yet built

- **The Build provider kind will invert the contract's direction.** A Build
  provider calls core for work rather than being called by it, because the thing
  that runs a build is a machine with a container runtime and requiring it to be
  reachable from core is the wrong trade. This is decided but unbuilt —
  [ADR-0004](docs/adr/0004-build-providers-call-core.md). If you are writing an
  **Infra** provider it does not affect you. If you were waiting to plug in your
  own build executor, read it and argue with it now rather than after.
- **The conformance suite will grow a second mode.** It proves a provider by
  calling a URL, which a Build provider does not have. Until that mode exists,
  the suite is the definition of a working *Infra* provider.
- **The Oracle reads will move out of core into an Infra provider of their
  own**, at which point core holds no cloud credentials on any substrate.

## [0.1.0] - 2026-08-29

First release, and the first under a licence. The headline is the published
`/v0` provider contract: TinyCloud reads infrastructure through providers, and
anyone can write one.

### Added

- **The `/v0` provider contract**, published as
  [`provider-contract-v0.yaml`](provider-contract-v0.yaml): one endpoint per
  capability of the Infra kind — instances, metrics, alarms, ingress, backups —
  plus capability discovery, `501` for a capability a provider does not serve,
  and bearer-token authentication. It carries **no stability promise**.
- **A Kubernetes Infra provider** needing no cloud account of any kind:
  instances from the node list, CPU and memory from metrics-server, the ingress
  address from the ingress service. `alarms` and `backups` return `501`, which
  is capability discovery working rather than a gap.
- **A provider server** (`cmd/provider-server`) hosting the in-tree providers
  over the same contract a third party would implement, so the maintainers' own
  providers exercise the public contract on every request.
- **A conformance suite** (`cmd/conformance`) that runs the contract against any
  provider URL and reports pass, fail or not-implemented per capability, exiting
  non-zero only on a genuine contract violation. It runs in CI against the
  in-tree providers.
- **Degradation that names what went wrong.** A provider that is unimplemented,
  erroring, timing out or unreachable each produce a rendered snapshot naming
  the failure; a previously good snapshot survives an outage and is flagged
  stale. No path produces a blank dashboard.
- `LICENSE` (Apache-2.0), `CONTRIBUTING.md`, `SECURITY.md`, this changelog, and
  issue templates — including one for telling us you are writing a provider,
  which is the most useful issue you can open.
- A domain glossary ([`CONTEXT.md`](CONTEXT.md)) and four ADRs.

### Changed

- **Core reads infrastructure through providers, not the Oracle SDK.** The
  vendor-named module inside core is gone; the HTTP provider client is one more
  implementation of the same source interfaces, so the collector, cache, infra
  endpoint, snapshot shape and console are unchanged.
- **Metrics are named, not queried in a vendor's language.** The contract takes
  `cpu.utilization` and a window rather than an Oracle MQL string —
  [ADR-0003](docs/adr/0003-contract-metrics-are-named-not-queried.md). The
  Kubernetes provider was built before the Oracle one specifically to find this,
  and found it.
- **There is no Registry provider kind.** The glossary listed three kinds and
  shipped one; core has no operation it would call a registry provider for that
  is not a string it already has. Like secrets and ingress, the registry is
  configuration. Two kinds remain: Infra and Build.
- The historical planning documents moved to
  [`docs/history/`](docs/history/), leaving `docs/` to describe what TinyCloud
  is rather than what happened in one lab.

### Removed

- **Every built-in default naming the maintainer's accounts.** The Oracle
  tenancy identifiers went first; this release removes the three that survived
  in the build plane — `BUILD_WORKFLOW_REPO`, `BUILD_COORDINATOR_PUBLIC_URL` and
  `IMAGE_PREFIX`, which defaulted to the maintainer's workflow repository, host
  and registry namespace. A published image no longer carries one operator's
  identity into anybody else's cluster.
- A compiled build-coordinator binary that was tracked in `bin/`, and a
  decommissioning script for a host terminated in 2026.

### Fixed

- The README claimed core holds no cloud credentials. It does hold Oracle
  credentials when an operator configures them, because the Oracle read path is
  still linked in. The README, the glossary and `docs/providers.md` now name
  that exception instead of contradicting the code.
- The maintainer's bastion and tenancy OCIDs were published in scripts and
  runbooks. They are parameterised. They remain in this repository's history.

### Upgrading

**If you run an instance, set these before deploying `v0.1.0`** — the build
coordinator no longer supplies them:

```
BUILD_WORKFLOW_REPO=your-account/tinycloud-platform
BUILD_COORDINATOR_PUBLIC_URL=https://tinycloud.example
IMAGE_PREFIX=ghcr.io/your-account
```

An unset value does not stop the coordinator from starting; it fails the build
that needs it, naming the variable. See
[`docs/deploy/build-coordinator.env.example`](docs/deploy/build-coordinator.env.example).

If you are on Oracle Cloud, `OCI_COMPARTMENT_ID`, `OCI_NLB_ID`,
`OCI_OBJECT_STORAGE_NAMESPACE` and `OCI_BACKUP_BUCKET` must be set in your
deployment or you lose alarms, ingress and backups from the dashboard.

[0.1.0]: https://github.com/sasiruLK/tinycloud-platform/releases/tag/v0.1.0

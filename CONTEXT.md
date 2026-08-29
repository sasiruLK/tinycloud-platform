# TinyCloud

A self-hostable platform for running applications on small, constrained
Kubernetes clusters. The reference substrate is an Oracle Cloud Always Free
tenancy, but the core depends on no cloud vendor: every vendor-specific
capability is reached through a provider.

## Language

### The platform

**TinyCloud**:
The platform as a whole: core, providers, and CLI together.
_Avoid_: the platform, the lab, the cluster

**Core**:
The vendor-neutral services — API, build coordinator, UI. Speaks to substrates
through providers, and holds no cloud credentials **except** the legacy Oracle
read path still linked into it, which is wired only when an operator supplies
that tenancy's identifiers and is scheduled to move out to a provider of its
own. Nothing about a tenancy is compiled in.
_Avoid_: the server, the backend, the control plane

**Instance**:
One deployment of TinyCloud, with its own GitOps repository and its own
providers. `gitops-lab` is the maintainer's instance, not a template.
_Avoid_: environment, installation, tenant

**Substrate**:
The cloud or cluster an instance runs on. Oracle Cloud Always Free is the
*reference substrate*: the one that is tested, not the only one supported.
_Avoid_: cloud, backend, infrastructure provider

**App**:
A user workload deployed through TinyCloud, one directory under `apps/` in an
instance's GitOps repository. TinyCloud's own manifests are not Apps.
_Avoid_: service, workload, project, deployment

### Providers

See `docs/providers.md` for the contract, how an instance is configured, and
how to write one.

**Provider**:
A service participating in the `/v0` contract for one kind, holding the
credentials for its substrate. Which side dials is a property of the kind, not
of providers in general: core calls an Infra provider, and a Build provider
calls core (ADR-0004). There is no term yet for that distinction, deliberately.
_Avoid_: plugin, adapter, driver, integration

**Provider kind**:
Which capability a provider serves. There are two: **Infra** (shipped) and
**Build** (specified in ADR-0004, not built). Secrets, ingress and the registry
are configuration, not provider kinds — core has no operation it would call a
registry provider for that is not "a string it already has".
_Avoid_: provider type, capability type

**Capability**:
One endpoint of the provider contract that a given provider declares it serves.
A provider may implement any subset; the rest return `501` and surface as
warnings.
_Avoid_: feature, method, endpoint

**Conformance suite**:
The published test binary that runs the full contract against a provider URL
and reports pass or fail per capability. The definition of a working **Infra**
provider; a Build provider has no URL to call, and proving one means a second
mode that does not exist yet (ADR-0004).
_Avoid_: test suite, validator, certification

**Release version**:
A version of this repository, tagged semver — `v0.1.0`. Moves whenever anything
here is released.

**Contract version**:
The `/v0` prefix every capability is served under. Independent of the release
version, and unchanged by most releases: per ADR-0001 it moves to `/v1` only
once a third-party provider exists to be broken by the change.
_Avoid_: the version, v0 (unqualified)

### Infrastructure

**Node names**:
`k3s-control`, `k3s-worker-1`, `amd-utility-1`, `amd-utility-2`. These are the
four instances Terraform manages.
_Avoid_: `build-vm`, `k3s-worker-2`, `monitoring-vm` — historical names for
hosts that no longer exist. Use them only in `docs/history/`, or in a record of
their own removal; never for anything current.

**Snapshot**:
One assembled view of an instance's infrastructure — nodes, alarms, ingress,
backups, capacity — cached with a TTL and served stale rather than missing.
_Avoid_: status, health, dashboard data

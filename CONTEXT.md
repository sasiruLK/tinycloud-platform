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
The vendor-neutral services — API, build coordinator, UI. Holds no cloud
credentials and speaks to substrates only through providers.
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

**Provider**:
An HTTP service implementing the `/v0` contract for one capability, deployed
alongside core and holding the credentials for its substrate.
_Avoid_: plugin, adapter, driver, integration

**Provider kind**:
Which capability a provider serves. There are three: **Infra**, **Registry**,
**Build**. Secrets and ingress are configuration, not provider kinds.
_Avoid_: provider type, capability type

**Capability**:
One endpoint of the provider contract that a given provider declares it serves.
A provider may implement any subset; the rest return `501` and surface as
warnings.
_Avoid_: feature, method, endpoint

**Conformance suite**:
The published test binary that runs the full contract against a provider URL
and reports pass or fail per capability. The definition of a working provider.
_Avoid_: test suite, validator, certification

### Infrastructure

**Node names**:
`k3s-control`, `k3s-worker-1`, `amd-utility-1`, `amd-utility-2`. These are the
four instances Terraform manages.
_Avoid_: `build-vm`, `k3s-worker-2`, `monitoring-vm` — historical names for
hosts that no longer exist. They appear only in `docs/history/`.

**Snapshot**:
One assembled view of an instance's infrastructure — nodes, alarms, ingress,
backups, capacity — cached with a TTL and served stale rather than missing.
_Avoid_: status, health, dashboard data

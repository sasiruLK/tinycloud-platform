---
status: accepted
---

# Contract metrics are named, not queried in a vendor's language

The five in-process source interfaces the collector reads through were meant to
be substrate-neutral, and four of them were. The fifth was not: `MetricSource`
took an Oracle Monitoring namespace and an MQL query string —
`CpuUtilization[5m].groupBy(resourceDisplayName).mean()` — and returned series
keyed by Oracle's dimension names. Publishing that as `/v0` would have required
every provider author to parse Oracle's query language, or to string-match the
five queries core happens to send.

The Kubernetes Infra Provider was built before the Oracle one precisely to find
this. It found it. The metric capability therefore takes a **name** the contract
defines — `cpu.utilization`, `memory.utilization`,
`ingress.healthy_backends`, `ingress.unhealthy_backends`,
`uptime.availability` — plus a window in seconds, and returns series whose
dimension keys are the contract's: `instance`, `backend_set`, `monitor`,
`target`. Translating a name into a substrate's own query language is the
provider's job, which is what a provider is for.

## Considered options

- **Ship MQL on the wire as-is.** Rejected: it makes "anyone can write a
  provider" mean "anyone who implements Oracle's query language". The Kubernetes
  provider could not have served the metric capability at all.
- **A general query language of our own.** Rejected: inventing a query language
  to avoid inheriting one is the larger mistake, and nothing in the snapshot
  needs more than five fixed metrics.
- **One endpoint per metric.** Rejected as a worse fit for capability
  discovery: a provider that serves utilisation but has no uptime monitoring
  would have to be described as serving four capabilities out of eight, when
  what it actually has is metrics minus the ones its substrate lacks.

## Consequences

- **A metric a substrate has no equivalent of returns no series, not an
  error.** Absent is not broken: the value reaches the UI as null, and no
  warning is raised for something nobody promised.
- **`MetricSource` changed shape inside core**, from
  `QueryMetric(ctx, namespace, query, window)` to
  `QueryMetric(ctx, metric, window)`. It is the only source interface this work
  changed, and the Oracle SDK reads translate the name back into a namespace
  and MQL at the edge, where the vendor-specific knowledge belongs.
- **The snapshot, the API response and the UI are unchanged.** This is a change
  to how a value is asked for, not to what is shown.
- **This is the falsification test ADR-0001 asked for, working as intended.**
  The contract changed before publication rather than after, which is the whole
  reason it ships as `/v0` with no stability promise.

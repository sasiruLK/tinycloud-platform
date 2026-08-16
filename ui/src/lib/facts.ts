/*
 * What this platform is, what it cannot do, and why.
 *
 * This file exists because every constraint below was learned the hard way and
 * then lived only in a chat log or a commit message. Six weeks later that is
 * indistinguishable from not knowing it at all — the OCIR migration was
 * designed, built and reverted because "OCIR is free" was believed rather than
 * checked.
 *
 * Verified against the tenancy on 2026-08-15. When something here changes,
 * change it here too: this page is only worth having if it is true.
 */

export interface Limit {
  name: string;
  value: string;
  note: string;
  severity?: "hard" | "watch";
}

export interface Blocked {
  service: string;
  evidence: string;
}

export interface Decision {
  title: string;
  choice: string;
  because: string;
}

export interface Gotcha {
  title: string;
  detail: string;
}

export const TOPOLOGY = [
  { name: "k3s-control", role: "control plane", spec: "ARM64 · 1 OCPU · 6 GB", note: "Argo CD, cert-manager, Traefik" },
  { name: "k3s-worker-1", role: "worker", spec: "ARM64 · 1 OCPU · 6 GB", note: "Workloads, build coordinator" },
  { name: "amd-utility-1", role: "utility", spec: "AMD64 · 1 OCPU · 1 GB", note: "Outside the critical path" },
  { name: "amd-utility-2", role: "utility", spec: "AMD64 · 1 OCPU · 1 GB", note: "Outside the critical path" },
];

export const LIMITS: Limit[] = [
  {
    name: "Idle free databases",
    value: "2 Autonomous DB + 1 MySQL HeatWave, all unprovisioned",
    severity: "watch",
    note: "The largest untouched allowance in the tenancy. MySQL HeatWave was previously recorded here as blocked, which was wrong: mysql-free-count and mysql-heatwave-free-count both show 1 available, in AD-1 only. The subnet is regional, so an AD-1 database is still reachable from the AD-3 cluster nodes. Nothing in the platform needs a database today except the build coordinator's SQLite file, which is pinned to one node. Provisioning either without a workload just creates something idle enough to be reclaimed.",
  },
  {
    name: "Ampere compute",
    value: "2 OCPU / 12 GB — fully used",
    severity: "hard",
    note: "Not the 4 OCPU / 24 GB usually quoted. Both nodes together consume the entire allowance, so no third VM can be created and autoscaling has nothing to scale into. Releasing a node to resize risks not getting the capacity back.",
  },
  {
    name: "Object Storage",
    value: "20 GiB, shared",
    severity: "watch",
    note: "One budget across every bucket, tier and archive. Backups are the main consumer.",
  },
  {
    name: "Block Volume backups",
    value: "5, tenancy-wide",
    severity: "hard",
    note: "The predefined bronze, silver and gold policies all exceed this and will start failing. A custom policy is required.",
  },
  {
    name: "APM probes",
    value: "10 executions per hour",
    severity: "hard",
    note: "Across all monitors combined. External vantage points count as three. This is why uptime checks run every 15 minutes rather than every minute.",
  },
  {
    name: "Logging ingest",
    value: "10 GB per month, shared with VCN flow logs",
    severity: "watch",
    note: "About 333 MB a day. Shipping all pod stdout would exhaust it; only platform events are sent.",
  },
  {
    name: "Network Load Balancer",
    value: "exactly 1",
    severity: "hard",
    note: "Spent on ingress. There is no second one for anything else.",
  },
];

export const BLOCKED: Blocked[] = [
  { service: "Container Registry (OCIR)", evidence: "403 FREE_TIER_NOT_SUPPORTED" },
  { service: "Health Checks", evidence: "monitor limits are 0" },
  { service: "Functions", evidence: "application-count 0" },
  { service: "API Gateway", evidence: "gateway-count 0" },
  { service: "Events", evidence: "rule-count 0" },
  { service: "DNS", evidence: "zone-count 0" },
  { service: "File Storage", evidence: "file-system-count 0" },
  { service: "Kubernetes (OKE)", evidence: "cluster-count 0" },
  { service: "Queue", evidence: "queues-max-count 0" },
  { service: "NoSQL", evidence: "0 read and write units, 0 GB table size" },
  { service: "PostgreSQL", evidence: "all 6 limits are 0" },
  { service: "OpenSearch / Redis", evidence: "node-count 0" },
  { service: "WAF", evidence: "policy-count 0" },
  { service: "NAT Gateway", evidence: "nat-gateway-count 0" },
  { service: "Log agent targeting", evidence: "only 0 groups are allowed" },
];

export const DECISIONS: Decision[] = [
  {
    title: "Registry",
    choice: "GHCR, not OCIR",
    because:
      "OCIR returns 403 FREE_TIER_NOT_SUPPORTED on this tenancy. An entire migration was built around it before that was verified, then reverted. GHCR is unlimited for public packages; the 500 MB cap applies only to private ones.",
  },
  {
    title: "Image tags",
    choice: "40-character commit SHAs",
    because:
      "The API once required semver while the image updater it generated only matched SHAs. The two sets were disjoint, so no onboarded app could ever receive an automated update. A test now binds them together.",
  },
  {
    title: "Where builds run",
    choice: "GitHub Actions",
    because:
      "Both ARM nodes are fully committed to k3s, so there is no host left to build on. Building in-cluster would put a memory-hungry job on the same 6 GB worker as the API — the build could evict the UI showing that build. The platform still owns the queue, logs and history; only the container build is external.",
  },
  {
    title: "Secrets",
    choice: "OCI Vault via instance principals",
    because:
      "SOPS was configured but nothing in the cluster could decrypt it, so it never worked. Instance principals mean there is no bootstrap credential at all — the nodes prove their own identity.",
  },
  {
    title: "Ingress",
    choice: "Traefik as a DaemonSet behind one network load balancer",
    because:
      "A single Traefik pod meant a node failure took ingress down for roughly five minutes while it rescheduled. A load balancer alone would have failed over instantly to a node with nothing serving.",
  },
];

export const GOTCHAS: Gotcha[] = [
  {
    title: "Sync is not Rebuild",
    detail:
      "Sync reconciles what git already says, so it deploys nothing new. Rebuild produces a new image from the latest commit on the app's branch — it is the only control that gets a code change onto the cluster. Until 2026-08-16 the second one did not exist: build_jobs.app_name was UNIQUE, so an app could be built exactly once and every later attempt returned 409. A rebuild is still a button, not something a push triggers.",
  },
  {
    title: "A zero limit outranks a working API",
    detail:
      "Health Checks answers list requests with HTTP 200 and an empty set while its monitor limit is 0. NoSQL reports an available environment with zero read and write units. Always check oci limits value list before designing around a service.",
  },
  {
    title: "NetworkPolicy egress to the Kubernetes API",
    detail:
      "kube-proxy rewrites the ClusterIP to the real API server address before policy is evaluated, so a rule naming 10.43.0.1:443 never matches and fails as connection refused. The node endpoint on 6443 must be allowed instead.",
  },
  {
    title: "Object Storage lists alphabetically, not by time",
    detail:
      "Using --limit with date-stamped object names returns the oldest objects. The backup validator did this and reported a months-old backup as latest every night for months.",
  },
  {
    title: "Traefik v3 dropped named-group HostRegexp",
    detail:
      "Rules written as HostRegexp(`{name:...}`) silently match nothing on Traefik 3. Application subdomains stopped routing without any error.",
  },
  {
    title: "Three namespaces, and they disagree",
    detail:
      "An Argo CD Application has a namespace of its own (always argocd), a declared destination namespace, and whatever its manifests actually pin. These agree rarely enough that all three have been wrong somewhere: the blog app declared one namespace while its manifests pinned another, the API's RBAC granted pod access in a namespace that had been deleted, and the resource graph searched for pods where the Application object lives rather than where its workloads run. When something reports nothing found, check which of the three it is using.",
  },
  {
    title: "Removing an Application deletes its workloads",
    detail:
      "Argo CD's resources-finalizer cascades. Excluding a path from an ApplicationSet makes it delete the Application it generated, which then takes the live Deployment and Service with it — the blog went down this way. Create the replacement Application first, let it adopt the resources, and only then remove the old owner.",
  },
  {
    title: "Expiring tokens fail silently",
    detail:
      "The GHCR token expired and every image build failed for a month with no signal, because nobody watches workflow runs. The daily health check exists specifically to catch this class of failure.",
  },
];

export const VERIFIED_ON = "2026-08-16";

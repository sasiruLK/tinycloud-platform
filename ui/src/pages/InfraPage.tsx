import { Cpu, HardDrive, Network, Archive, Globe } from "lucide-react";
import { useInfra } from "@/hooks/useInfra";
import {Dot, Pill, Freshness, Meter} from "@/components/Signal";
import { type Level } from "@/lib/level";
import type { InfraAlarm, InfraNode } from "@/types/api";

/**
 * The layer beneath Argo CD. Everything here comes from OCI directly — Compute,
 * Monitoring, the load balancer and Object Storage — so a problem in the
 * substrate is visible without leaving the platform for the OCI console.
 */
export function InfraPage() {
  const { infra, loading, error } = useInfra();

  if (loading && !infra) {
    return <p className="py-16 text-center font-mono text-sm text-[var(--color-muted)]">Reading OCI…</p>;
  }

  if (!infra) {
    return (
      <div className="rounded-[var(--radius-md)] border border-[var(--color-crit)]/25 bg-[var(--color-crit-wash)] p-4">
        <p className="font-mono text-sm text-[var(--color-crit)]">Infrastructure unavailable</p>
        <p className="mt-1 text-xs text-[var(--color-ink-2)]">{error}</p>
        <p className="mt-2 text-[11px] text-[var(--color-muted)]">
          The API reads OCI with the node's instance principal. If this persists, the most likely cause
          is a missing IAM policy rather than an outage.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="font-mono text-lg text-[var(--color-ink)]">Infrastructure</h1>
        <div className="flex items-center gap-3">
          {infra.stale && (
            <Pill level="warn">stale — showing last good read</Pill>
          )}
          <span className="font-mono text-[11px] text-[var(--color-muted)]">
            updated <Freshness at={infra.updatedAt} warnAfterMins={5} critAfterMins={30} />
          </span>
        </div>
      </header>

      {/* --- nodes ------------------------------------------------------- */}
      <Section icon={Cpu} title="Compute">
        <div className="grid gap-3 md:grid-cols-2">
          {infra.nodes.map((n) => (
            <NodeCard key={n.name} node={n} />
          ))}
        </div>
      </Section>

      {/* --- ingress ----------------------------------------------------- */}
      <Section icon={Network} title="Ingress">
        <div className="flex flex-wrap items-center gap-x-8 gap-y-3 rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] p-4">
          <Field label="Public IP" value={infra.ingress.publicIp || "—"} mono />
          <Field
            label="Healthy backends"
            value={infra.ingress.healthyBackends ?? "—"}
            level={
              infra.ingress.healthyBackends == null
                ? "idle"
                : infra.ingress.healthyBackends >= 2
                  ? "ok"
                  : infra.ingress.healthyBackends === 1
                    ? "warn"
                    : "crit"
            }
          />
          <Field
            label="Unhealthy"
            value={infra.ingress.unhealthyBackends ?? "—"}
            level={infra.ingress.unhealthyBackends ? "crit" : "ok"}
          />
          <p className="text-[11px] text-[var(--color-faint)]">
            Two healthy backends means either node can fail without an outage. One means redundancy is
            gone, though the site is still up.
          </p>
        </div>
      </Section>

      {/* --- uptime ------------------------------------------------------ */}
      {infra.uptime.length > 0 && (
        <Section icon={Globe} title="External probes">
          <div className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
            <table className="w-full text-sm">
              <tbody className="divide-y divide-[var(--color-line-soft)]">
                {infra.uptime.map((u) => (
                  <tr key={u.monitor}>
                    <td className="px-3 py-2 font-mono text-xs">{u.monitor}</td>
                    <td className="truncate px-3 py-2 font-mono text-[11px] text-[var(--color-muted)]">
                      {u.target}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <Pill level={u.availability === null ? "idle" : u.availability >= 1 ? "ok" : "crit"}>
                        {u.availability === null ? "no data" : u.availability >= 1 ? "reachable" : "down"}
                      </Pill>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="border-t border-[var(--color-line)] px-3 py-2 text-[11px] text-[var(--color-faint)]">
              Probed from outside OCI every 15 minutes. The free tier allows 10 probe executions per
              hour in total, which is why the interval is not shorter.
            </p>
          </div>
        </Section>
      )}

      {/* --- alarms ------------------------------------------------------ */}
      <Section icon={HardDrive} title="Alarms">
        <div className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <tbody className="divide-y divide-[var(--color-line-soft)]">
              {infra.alarms.map((a) => (
                <tr key={a.name}>
                  <td className="w-6 pl-3">
                    <Dot level={alarmLevel(a)} />
                  </td>
                  <td className="px-2 py-2 font-mono text-xs">{a.name}</td>
                  <td className="px-2 py-2 font-mono text-[10px] uppercase text-[var(--color-muted)]">
                    {a.severity}
                  </td>
                  <td className="px-3 py-2 text-right font-mono text-[11px] text-[var(--color-ink-2)]">
                    {a.status}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      {/* --- backups ----------------------------------------------------- */}
      <Section icon={Archive} title="Backups">
        <div className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-line)] text-left font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
                <th className="px-3 py-1.5 font-normal">Stream</th>
                <th className="px-3 py-1.5 font-normal">Objects</th>
                <th className="px-3 py-1.5 text-right font-normal">Newest</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--color-line-soft)]">
              {infra.backups.streams.map((s) => (
                <tr key={s.prefix}>
                  <td className="px-3 py-2 font-mono text-xs">{s.prefix}</td>
                  <td className="px-3 py-2 font-mono text-xs tabular text-[var(--color-ink-2)]">
                    {s.count}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <Freshness at={s.newest} warnAfterMins={26 * 60} critAfterMins={3 * 24 * 60} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="border-t border-[var(--color-line)] px-3 py-2">
            <div className="mb-1 flex items-baseline justify-between">
              <span className="font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
                Bucket {infra.backups.bucket}
              </span>
              <span className="font-mono text-[11px] tabular text-[var(--color-ink-2)]">
                {(infra.backups.sizeBytes / 1024 ** 3).toFixed(2)} GiB ·{" "}
                {infra.backups.objectCount} objects
              </span>
            </div>
            <Meter
              used={infra.capacity.objectStorageUsedBytes}
              total={infra.capacity.objectStorageTotalBytes}
            />
          </div>
        </div>
      </Section>
    </div>
  );
}

function NodeCard({ node }: { node: InfraNode }) {
  const up = node.state === "RUNNING";
  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] p-3">
      <div className="mb-3 flex items-center gap-2">
        <Dot level={up ? "ok" : "crit"} live={up} />
        <span className="font-mono text-sm">{node.name}</span>
        <span className="rounded-[var(--radius-sm)] bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[10px] uppercase text-[var(--color-muted)]">
          {node.role}
        </span>
        <span className="ml-auto font-mono text-[11px] text-[var(--color-muted)]">{node.privateIp}</span>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <Gauge label="CPU" pct={node.cpuPercent} />
        <Gauge label="Memory" pct={node.memoryPercent} />
      </div>

      <dl className="mt-3 flex flex-wrap gap-x-4 gap-y-1 border-t border-[var(--color-line-soft)] pt-2 font-mono text-[10px] text-[var(--color-faint)]">
        <span>{node.shape}</span>
        <span>
          {node.ocpus} OCPU · {node.memoryGb} GB
        </span>
        <span>{node.faultDomain}</span>
      </dl>
    </div>
  );
}

function Gauge({ label, pct }: { label: string; pct: number | null }) {
  const level: Level = pct == null ? "idle" : pct >= 90 ? "crit" : pct >= 75 ? "warn" : "ok";
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between">
        <span className="font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
          {label}
        </span>
        <span className="font-mono text-xs tabular text-[var(--color-ink-2)]">
          {pct == null ? "unknown" : `${pct.toFixed(0)}%`}
        </span>
      </div>
      <Meter used={pct ?? 0} total={100} level={level} />
    </div>
  );
}

function Field({
  label,
  value,
  mono,
  level,
}: {
  label: string;
  value: React.ReactNode;
  mono?: boolean;
  level?: Level;
}) {
  const tone =
    level === "crit"
      ? "text-[var(--color-crit)]"
      : level === "warn"
        ? "text-[var(--color-warn)]"
        : level === "ok"
          ? "text-[var(--color-ok)]"
          : "text-[var(--color-ink)]";
  return (
    <div className="flex flex-col gap-0.5">
      <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
        {label}
      </span>
      <span className={`text-sm tabular ${mono ? "font-mono" : ""} ${tone}`}>{value}</span>
    </div>
  );
}

function Section({
  icon: Icon,
  title,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="flex items-center gap-2 font-mono text-[11px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
        <Icon className="h-3.5 w-3.5" />
        {title}
      </h2>
      {children}
    </section>
  );
}

function alarmLevel(a: InfraAlarm): Level {
  if (a.status?.toUpperCase() === "FIRING") {
    return a.severity?.toUpperCase() === "CRITICAL" ? "crit" : "warn";
  }
  if (a.status?.toUpperCase() === "SUSPENDED") return "idle";
  return "ok";
}

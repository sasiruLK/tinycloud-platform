import { Link } from "react-router-dom";
import { AlertTriangle, ArrowRight, ShieldCheck } from "lucide-react";
import { useApps } from "@/hooks/useApps";
import { useInfra } from "@/hooks/useInfra";
import {Dot, Pill, Freshness, Stat, Meter} from "@/components/Signal";
import { levelOf, type Level } from "@/lib/level";
import type { InfraAlarm } from "@/types/api";

/**
 * The landing page exists to answer one question — is anything wrong — without
 * the reader opening a log, a workflow run, or a chat transcript. Everything
 * that has ever failed silently on this platform gets a line here.
 */
export function OverviewPage() {
  const { apps } = useApps();
  const { infra, error: infraError } = useInfra();

  const firing = (infra?.alarms ?? []).filter((a) => a.status?.toUpperCase() === "FIRING");
  const unhealthyApps = apps.filter(
    (a) => levelOf(a.health) === "crit" || levelOf(a.syncStatus) === "crit",
  );
  const driftedApps = apps.filter(
    (a) => levelOf(a.health) === "warn" || levelOf(a.syncStatus) === "warn",
  );

  const allWell = firing.length === 0 && unhealthyApps.length === 0 && !infraError;

  return (
    <div className="flex flex-col gap-6">
      <Verdict
        ok={allWell}
        firing={firing}
        broken={unhealthyApps.length}
        drifted={driftedApps.length}
        infraError={infraError}
      />

      <section className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <Panel>
          <Stat label="Apps" value={apps.length} level={apps.length ? "ok" : "idle"} />
        </Panel>
        <Panel>
          <Stat
            label="Healthy"
            value={apps.filter((a) => levelOf(a.health) === "ok").length}
            level="ok"
          />
        </Panel>
        <Panel>
          <Stat
            label="Alarms firing"
            value={firing.length}
            level={firing.length ? "crit" : "ok"}
            hint="OCI Monitoring alarms currently in breach"
          />
        </Panel>
        <Panel>
          <Stat
            label="Nodes up"
            value={
              infra ? `${infra.nodes.filter((n) => n.state === "RUNNING").length}/${infra.nodes.length}` : "—"
            }
            level={
              infra && infra.nodes.some((n) => n.state !== "RUNNING") ? "crit" : infra ? "ok" : "idle"
            }
          />
        </Panel>
        <Panel>
          <Stat
            label="Ingress backends"
            value={
              infra?.ingress?.healthyBackends != null
                ? `${infra.ingress.healthyBackends}`
                : "—"
            }
            level={
              infra?.ingress?.unhealthyBackends
                ? "warn"
                : infra?.ingress?.healthyBackends
                  ? "ok"
                  : "idle"
            }
            hint="Nodes answering the load balancer. Below 2 means redundancy is gone."
          />
        </Panel>
        <Panel>
          <Stat
            label="Backup age"
            value={
              newestBackup(infra) ? (
                <Freshness at={newestBackup(infra)} warnAfterMins={26 * 60} critAfterMins={3 * 24 * 60} />
              ) : (
                "—"
              )
            }
            hint="Newest object across all backup streams. Turns amber past 26h."
          />
        </Panel>
      </section>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Card title="Applications" href="/apps" action="All apps">
            {apps.length === 0 ? (
              <Empty>Nothing deployed yet.</Empty>
            ) : (
              <ul className="divide-y divide-[var(--color-line-soft)]">
                {apps.slice(0, 6).map((a) => (
                  <li key={a.name}>
                    <Link
                      to={`/apps/${a.name}`}
                      className="flex items-center gap-3 px-3 py-2 transition-colors hover:bg-[var(--color-surface-2)]"
                    >
                      <Dot level={levelOf(a.health)} />
                      <span className="min-w-0 flex-1 truncate font-mono text-sm">{a.name}</span>
                      <Pill level={levelOf(a.syncStatus)}>{a.syncStatus}</Pill>
                      <code className="hidden text-[11px] text-[var(--color-muted)] sm:inline">
                        {a.imageTag?.slice(0, 7)}
                      </code>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        <Card title="Alarms" href="/infra" action="Infra">
          {!infra ? (
            <Empty>{infraError ?? "Loading…"}</Empty>
          ) : (
            <ul className="divide-y divide-[var(--color-line-soft)]">
              {infra.alarms.map((al) => (
                <li key={al.name} className="flex items-center gap-2 px-3 py-2">
                  <Dot level={alarmLevel(al)} />
                  <span className="min-w-0 flex-1 truncate font-mono text-xs text-[var(--color-ink-2)]">
                    {al.name}
                  </span>
                  <span className="font-mono text-[10px] uppercase text-[var(--color-muted)]">
                    {al.status}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {infra && (
        <Card title="Capacity" href="/system" action="Why these limits">
          <div className="grid gap-4 p-3 sm:grid-cols-2">
            <CapacityRow
              label="Ampere OCPU"
              used={infra.capacity.ampereOcpuUsed}
              total={infra.capacity.ampereOcpuTotal}
              render={(u, t) => `${u} / ${t}`}
              note="Fully allocated. No headroom for another VM."
            />
            <CapacityRow
              label="Object storage"
              used={infra.capacity.objectStorageUsedBytes}
              total={infra.capacity.objectStorageTotalBytes}
              render={(u, t) => `${gib(u)} / ${gib(t)} GiB`}
              note="Shared across backups, archives and every other bucket."
            />
          </div>
        </Card>
      )}
    </div>
  );
}

function Verdict({
  ok,
  firing,
  broken,
  drifted,
  infraError,
}: {
  ok: boolean;
  firing: InfraAlarm[];
  broken: number;
  drifted: number;
  infraError: string | null;
}) {
  if (ok) {
    return (
      <div className="flex items-center gap-3 rounded-[var(--radius-md)] border border-[var(--color-ok)]/25 bg-[var(--color-ok-wash)] px-4 py-3">
        <ShieldCheck className="h-5 w-5 shrink-0 text-[var(--color-ok)]" />
        <div>
          <p className="font-mono text-sm text-[var(--color-ok)]">Everything is healthy</p>
          <p className="text-xs text-[var(--color-muted)]">
            No alarms firing, every application synced and healthy.
          </p>
        </div>
      </div>
    );
  }

  const lines = [
    firing.length > 0 && `${firing.length} alarm${firing.length > 1 ? "s" : ""} firing`,
    broken > 0 && `${broken} application${broken > 1 ? "s" : ""} unhealthy`,
    drifted > 0 && `${drifted} drifted or degraded`,
    infraError && `infrastructure unreachable: ${infraError}`,
  ].filter(Boolean) as string[];

  return (
    <div className="flex items-center gap-3 rounded-[var(--radius-md)] border border-[var(--color-crit)]/25 bg-[var(--color-crit-wash)] px-4 py-3">
      <AlertTriangle className="h-5 w-5 shrink-0 text-[var(--color-crit)]" />
      <div>
        <p className="font-mono text-sm text-[var(--color-crit)]">Attention needed</p>
        <p className="text-xs text-[var(--color-ink-2)]">{lines.join(" · ")}</p>
      </div>
    </div>
  );
}

function Panel({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] px-3 py-2.5">
      {children}
    </div>
  );
}

function Card({
  title,
  href,
  action,
  children,
}: {
  title: string;
  href?: string;
  action?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
      <header className="flex items-center justify-between border-b border-[var(--color-line)] px-3 py-2">
        <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
          {title}
        </h2>
        {href && action && (
          <Link
            to={href}
            className="flex items-center gap-1 font-mono text-[11px] text-[var(--color-accent)] hover:underline"
          >
            {action}
            <ArrowRight className="h-3 w-3" />
          </Link>
        )}
      </header>
      {children}
    </section>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="px-3 py-6 text-center font-mono text-xs text-[var(--color-muted)]">{children}</p>;
}

function CapacityRow({
  label,
  used,
  total,
  render,
  note,
}: {
  label: string;
  used: number;
  total: number;
  render: (u: number, t: number) => string;
  note: string;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between">
        <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
          {label}
        </span>
        <span className="font-mono text-xs tabular text-[var(--color-ink-2)]">{render(used, total)}</span>
      </div>
      <Meter used={used} total={total} />
      <p className="text-[11px] text-[var(--color-faint)]">{note}</p>
    </div>
  );
}

function alarmLevel(a: InfraAlarm): Level {
  if (a.status?.toUpperCase() === "FIRING") {
    return a.severity?.toUpperCase() === "CRITICAL" ? "crit" : "warn";
  }
  if (a.status?.toUpperCase() === "SUSPENDED") return "idle";
  return "ok";
}

function newestBackup(infra: ReturnType<typeof useInfra>["infra"]): string | null {
  const times = (infra?.backups?.streams ?? []).map((s) => s.newest).filter(Boolean) as string[];
  return times.length ? times.sort().reverse()[0] : null;
}

function gib(bytes: number): string {
  return (bytes / 1024 ** 3).toFixed(1);
}

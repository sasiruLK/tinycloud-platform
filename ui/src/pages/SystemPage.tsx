import { Ban, Lightbulb, Ruler, Server, TriangleAlert } from "lucide-react";
import { BLOCKED, DECISIONS, GOTCHAS, LIMITS, TOPOLOGY, VERIFIED_ON } from "@/lib/facts";

/**
 * The knowledge layer.
 *
 * Everything a dashboard normally leaves in someone's head: what the hardware
 * is, which limits are hard, which services are unavailable despite being
 * advertised, and why each significant decision went the way it did.
 */
export function SystemPage() {
  return (
    <div className="flex flex-col gap-8">
      <header className="flex flex-col gap-1">
        <h1 className="font-mono text-lg">System</h1>
        <p className="max-w-[65ch] text-sm text-[var(--color-ink-2)]">
          What this platform is, what it cannot do, and why it is built this way. Every limit below was
          read from the tenancy rather than from documentation — the free-tier page is wrong about most
          of this account.
        </p>
        <p className="font-mono text-[11px] text-[var(--color-faint)]">verified {VERIFIED_ON}</p>
      </header>

      <Section icon={Server} title="Topology">
        <div className="overflow-x-auto rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-line)] text-left font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
                <th className="px-3 py-1.5 font-normal">Node</th>
                <th className="px-3 py-1.5 font-normal">Role</th>
                <th className="px-3 py-1.5 font-normal">Spec</th>
                <th className="px-3 py-1.5 font-normal">Runs</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--color-line-soft)]">
              {TOPOLOGY.map((n) => (
                <tr key={n.name}>
                  <td className="whitespace-nowrap px-3 py-2 font-mono text-xs">{n.name}</td>
                  <td className="whitespace-nowrap px-3 py-2 font-mono text-[11px] text-[var(--color-muted)]">
                    {n.role}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 font-mono text-[11px] text-[var(--color-ink-2)]">
                    {n.spec}
                  </td>
                  <td className="px-3 py-2 text-xs text-[var(--color-muted)]">{n.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Section>

      <Section icon={Ruler} title="Hard limits">
        <div className="grid gap-2 md:grid-cols-2">
          {LIMITS.map((l) => (
            <div
              key={l.name}
              className={`rounded-[var(--radius-md)] border bg-[var(--color-surface)] p-3 ${
                l.severity === "hard"
                  ? "border-[var(--color-crit)]/25"
                  : "border-[var(--color-line)]"
              }`}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span className="font-mono text-xs text-[var(--color-ink)]">{l.name}</span>
                <span
                  className={`font-mono text-[11px] tabular ${
                    l.severity === "hard" ? "text-[var(--color-crit)]" : "text-[var(--color-warn)]"
                  }`}
                >
                  {l.value}
                </span>
              </div>
              <p className="mt-1.5 text-[11px] leading-relaxed text-[var(--color-muted)]">{l.note}</p>
            </div>
          ))}
        </div>
      </Section>

      <Section icon={Ban} title="Unavailable on this tenancy">
        <p className="mb-2 max-w-[65ch] text-xs text-[var(--color-muted)]">
          All advertised as Always Free. All verified blocked. Do not design around any of them — this
          is the mistake that cost the registry migration.
        </p>
        <div className="flex flex-wrap gap-1.5">
          {BLOCKED.map((b) => (
            <span
              key={b.service}
              title={b.evidence}
              className="rounded-[var(--radius-sm)] border border-[var(--color-crit)]/20 bg-[var(--color-crit-wash)] px-2 py-1 font-mono text-[11px] text-[var(--color-crit)]"
            >
              {b.service}
            </span>
          ))}
        </div>
      </Section>

      <Section icon={Lightbulb} title="Why it is built this way">
        <div className="flex flex-col gap-2">
          {DECISIONS.map((d) => (
            <div
              key={d.title}
              className="rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] p-3"
            >
              <div className="flex flex-wrap items-baseline gap-2">
                <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
                  {d.title}
                </span>
                <span className="font-mono text-xs text-[var(--color-accent)]">{d.choice}</span>
              </div>
              <p className="mt-1.5 max-w-[75ch] text-[11px] leading-relaxed text-[var(--color-ink-2)]">
                {d.because}
              </p>
            </div>
          ))}
        </div>
      </Section>

      <Section icon={TriangleAlert} title="Things that will bite you again">
        <ol className="flex flex-col gap-2">
          {GOTCHAS.map((g, i) => (
            <li
              key={g.title}
              className="flex gap-3 rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)] p-3"
            >
              <span className="font-mono text-xs tabular text-[var(--color-accent)]">
                {String(i + 1).padStart(2, "0")}
              </span>
              <div>
                <p className="font-mono text-xs text-[var(--color-ink)]">{g.title}</p>
                <p className="mt-1 max-w-[75ch] text-[11px] leading-relaxed text-[var(--color-muted)]">
                  {g.detail}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </Section>
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

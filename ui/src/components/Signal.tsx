/*
 * Status primitives.
 *
 * Every silent failure this platform has had was correctly reported somewhere
 * nobody looked. These exist so state is legible at a glance rather than read:
 * a dot encodes severity in colour AND shape-of-motion, and Freshness turns
 * "when did this last succeed" into something that goes red on its own.
 */
import { useEffect, useState } from "react";
import type { Level } from "@/lib/level";

export type { Level };

const TONE: Record<Level, { fg: string; bg: string; label: string }> = {
  ok: { fg: "text-[var(--color-ok)]", bg: "bg-[var(--color-ok-wash)]", label: "border-[var(--color-ok)]/30" },
  warn: { fg: "text-[var(--color-warn)]", bg: "bg-[var(--color-warn-wash)]", label: "border-[var(--color-warn)]/30" },
  crit: { fg: "text-[var(--color-crit)]", bg: "bg-[var(--color-crit-wash)]", label: "border-[var(--color-crit)]/30" },
  info: { fg: "text-[var(--color-info)]", bg: "bg-[var(--color-info-wash)]", label: "border-[var(--color-info)]/30" },
  idle: { fg: "text-[var(--color-muted)]", bg: "bg-[var(--color-surface-2)]", label: "border-[var(--color-line)]" },
};

export function Dot({ level, live = false }: { level: Level; live?: boolean }) {
  return (
    <span className="relative inline-flex h-2 w-2 shrink-0">
      {live && <span className={`pulse-ring absolute inset-0 ${TONE[level].fg}`} />}
      <span className={`h-2 w-2 rounded-full bg-current ${TONE[level].fg}`} />
    </span>
  );
}

export function Pill({
  level,
  children,
  live = false,
}: {
  level: Level;
  children: React.ReactNode;
  live?: boolean;
}) {
  const t = TONE[level];
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-[var(--radius-sm)] border px-1.5 py-0.5 font-mono text-[11px] uppercase tracking-wide ${t.bg} ${t.fg} ${t.label}`}
    >
      <Dot level={level} live={live} />
      {children}
    </span>
  );
}

/**
 * How long ago something last succeeded, and whether that is now a problem.
 *
 * This is the component that would have surfaced the month-dead GHCR token and
 * the stale gitops backup. Pass the threshold at which silence becomes
 * suspicious; it colours itself.
 */
export function Freshness({
  at,
  warnAfterMins = 60 * 26,
  critAfterMins = 60 * 24 * 3,
  prefix,
}: {
  at: string | null | undefined;
  warnAfterMins?: number;
  critAfterMins?: number;
  prefix?: string;
}) {
  // The clock lives in state rather than being read during render: rendering
  // must be pure, and this component re-renders on its own schedule anyway.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);

  if (!at) return <span className="font-mono text-xs text-[var(--color-crit)]">never</span>;

  const mins = Math.floor((now - new Date(at).getTime()) / 60000);
  const level: Level = mins >= critAfterMins ? "crit" : mins >= warnAfterMins ? "warn" : "ok";

  return (
    <span className={`font-mono text-xs ${TONE[level].fg}`} title={new Date(at).toISOString()}>
      {prefix ? `${prefix} ` : ""}
      {relative(mins)}
    </span>
  );
}

function relative(mins: number): string {
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const h = Math.floor(mins / 60);
  if (h < 48) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

/** A labelled figure. Used wherever a number needs a unit and a caption. */
export function Stat({
  label,
  value,
  unit,
  level = "idle",
  hint,
}: {
  label: string;
  value: React.ReactNode;
  unit?: string;
  level?: Level;
  hint?: string;
}) {
  return (
    <div className="flex flex-col gap-0.5" title={hint}>
      <span className="font-mono text-[10px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
        {label}
      </span>
      <span className={`font-mono text-xl leading-none tabular ${TONE[level].fg}`}>
        {value}
        {unit && <span className="ml-0.5 text-xs text-[var(--color-muted)]">{unit}</span>}
      </span>
    </div>
  );
}

/** Proportion bar for capacity that is genuinely finite — the Ampere cap, the 20 GiB. */
export function Meter({ used, total, level }: { used: number; total: number; level?: Level }) {
  const pct = total > 0 ? Math.min(100, (used / total) * 100) : 0;
  const auto: Level = pct >= 90 ? "crit" : pct >= 75 ? "warn" : "ok";
  const l = level ?? auto;
  return (
    <div className="h-1 w-full overflow-hidden rounded-full bg-[var(--color-surface-2)]">
      <div
        className={`h-full rounded-full bg-current transition-[width] duration-500 ${TONE[l].fg}`}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

import { useEffect, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import { RefreshCw } from "lucide-react";
import { Dot, Pill, Freshness } from "@/components/Signal";
import { levelOf } from "@/lib/level";
import { ErrorAlert } from "@/components/ErrorAlert";
import type { BuildJob } from "@/types/api";

const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api";

/**
 * Deploy history.
 *
 * Every build this platform has run was already recorded, but reachable only if
 * you happened to know its UUID — so in practice the history did not exist.
 * This is the index that makes it findable.
 */
export function BuildsPage() {
  const [builds, setBuilds] = useState<BuildJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetch_ = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/v1/builds?limit=50`, { credentials: "include" });
      if (!res.ok) {
        const b = await res.json().catch(() => null);
        throw new Error(b?.message || `HTTP ${res.status}`);
      }
      const body = await res.json();
      setBuilds(body.data?.builds ?? body.builds ?? []);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load builds");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetch_();
    // Faster than the 30s used elsewhere: a build in flight changes state on
    // the order of seconds and this is the page you watch while it runs.
    const t = setInterval(fetch_, 10_000);
    return () => clearInterval(t);
  }, [fetch_]);

  const running = builds.filter((b) => b.status === "running" || b.status === "queued").length;

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-mono text-lg">Builds</h1>
          <p className="font-mono text-[11px] text-[var(--color-muted)]">
            {builds.length} recent · {running > 0 ? `${running} in flight` : "none running"}
          </p>
        </div>
        <button
          onClick={fetch_}
          className="flex items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--color-line)] px-2.5 py-1.5 font-mono text-xs text-[var(--color-ink-2)] transition-colors hover:bg-[var(--color-surface-2)]"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          Refresh
        </button>
      </header>

      {error && <ErrorAlert message={error} onRetry={fetch_} />}

      {!error && builds.length === 0 && !loading && (
        <div className="rounded-[var(--radius-md)] border border-dashed border-[var(--color-line)] px-4 py-16 text-center">
          <p className="font-mono text-sm text-[var(--color-ink-2)]">No builds yet</p>
          <p className="mt-1 text-xs text-[var(--color-muted)]">
            Deploying an app dispatches a build to GitHub Actions and streams its log back here.
          </p>
        </div>
      )}

      {builds.length > 0 && (
        <div className="overflow-x-auto rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-line)] text-left font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
                <th className="w-8" />
                <th className="px-2 py-2 font-normal">App</th>
                <th className="px-2 py-2 font-normal">Status</th>
                <th className="hidden px-2 py-2 font-normal sm:table-cell">Commit</th>
                <th className="hidden px-2 py-2 font-normal md:table-cell">Framework</th>
                <th className="hidden px-2 py-2 font-normal lg:table-cell">Took</th>
                <th className="px-2 py-2 text-right font-normal">Started</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--color-line-soft)]">
              {builds.map((b) => {
                const live = b.status === "running" || b.status === "queued";
                return (
                  <tr
                    key={b.id}
                    className="group cursor-pointer transition-colors hover:bg-[var(--color-surface-2)]"
                  >
                    <td className="pl-3">
                      <Dot level={levelOf(b.status)} live={live} />
                    </td>
                    <td className="px-2 py-2">
                      <Link to={`/builds/${b.id}`} className="font-mono text-xs">
                        <span className="text-[var(--color-ink)] group-hover:text-[var(--color-accent)]">
                          {b.appName}
                        </span>
                      </Link>
                    </td>
                    <td className="px-2 py-2">
                      <Pill level={levelOf(b.status)} live={live}>
                        {b.status}
                      </Pill>
                    </td>
                    <td className="hidden px-2 py-2 sm:table-cell">
                      <code className="font-mono text-[11px] text-[var(--color-muted)]">
                        {b.commitSha ? b.commitSha.slice(0, 7) : "—"}
                      </code>
                    </td>
                    <td className="hidden px-2 py-2 font-mono text-[11px] text-[var(--color-muted)] md:table-cell">
                      {b.framework || "—"}
                    </td>
                    <td className="hidden px-2 py-2 font-mono text-[11px] tabular text-[var(--color-muted)] lg:table-cell">
                      {duration(b)}
                    </td>
                    <td className="px-2 py-2 text-right">
                      <Freshness
                        at={b.createdAt}
                        warnAfterMins={Number.MAX_SAFE_INTEGER}
                        critAfterMins={Number.MAX_SAFE_INTEGER}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** Wall-clock time a build took, or how long it has been running. */
function duration(b: BuildJob): string {
  const start = b.startedAt ?? b.createdAt;
  if (!start) return "—";
  const end = b.finishedAt ? new Date(b.finishedAt).getTime() : Date.now();
  const secs = Math.max(0, Math.round((end - new Date(start).getTime()) / 1000));
  if (secs < 90) return `${secs}s`;
  return `${Math.floor(secs / 60)}m ${secs % 60}s`;
}

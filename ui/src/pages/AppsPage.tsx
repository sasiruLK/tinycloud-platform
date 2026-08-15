import { Link } from "react-router-dom";
import { Plus, RefreshCw } from "lucide-react";
import { useApps } from "@/hooks/useApps";
import { ErrorAlert } from "@/components/ErrorAlert";
import {Dot, Pill} from "@/components/Signal";
import { levelOf } from "@/lib/level";

/**
 * A table, not a card grid. Cards looked fine at four apps and wasted the
 * screen at twenty; a row per app scans faster and puts health, sync and image
 * on one line where they can be compared down a column.
 */
export function AppsPage() {
  const { apps, loading, error, errorRequestId, refetch } = useApps();

  return (
    <div className="flex flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="font-mono text-lg">Applications</h1>
          <p className="font-mono text-[11px] text-[var(--color-muted)]">
            {apps.length} deployed · refreshes every 30s
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={refetch}
            className="flex items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--color-line)] px-2.5 py-1.5 font-mono text-xs text-[var(--color-ink-2)] transition-colors hover:bg-[var(--color-surface-2)]"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </button>
          <Link
            to="/apps/new"
            className="flex items-center gap-1.5 rounded-[var(--radius-sm)] bg-[var(--color-accent)] px-2.5 py-1.5 font-mono text-xs font-medium text-[#1a1200] transition-opacity hover:opacity-90"
          >
            <Plus className="h-3.5 w-3.5" />
            Deploy app
          </Link>
        </div>
      </header>

      {error && <ErrorAlert message={error} requestId={errorRequestId} onRetry={refetch} />}

      {!error && apps.length === 0 && !loading && (
        <div className="rounded-[var(--radius-md)] border border-dashed border-[var(--color-line)] px-4 py-16 text-center">
          <p className="font-mono text-sm text-[var(--color-ink-2)]">No applications yet</p>
          <p className="mt-1 text-xs text-[var(--color-muted)]">
            Deploying one builds it in GitHub Actions, pushes to GHCR and commits the manifests here.
          </p>
          <Link
            to="/apps/new"
            className="mt-4 inline-flex items-center gap-1.5 rounded-[var(--radius-sm)] bg-[var(--color-accent)] px-3 py-1.5 font-mono text-xs text-[#1a1200]"
          >
            <Plus className="h-3.5 w-3.5" />
            Deploy your first app
          </Link>
        </div>
      )}

      {apps.length > 0 && (
        <div className="overflow-x-auto rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-line)] text-left font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
                <th className="w-8" />
                <th className="px-2 py-2 font-normal">Name</th>
                <th className="px-2 py-2 font-normal">Health</th>
                <th className="px-2 py-2 font-normal">Sync</th>
                <th className="hidden px-2 py-2 font-normal md:table-cell">Image</th>
                <th className="hidden px-2 py-2 font-normal lg:table-cell">Repository</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--color-line-soft)]">
              {apps.map((a) => (
                <tr
                  key={a.name}
                  className="group cursor-pointer transition-colors hover:bg-[var(--color-surface-2)]"
                >
                  <td className="pl-3">
                    <Dot level={levelOf(a.health)} />
                  </td>
                  <td className="px-2 py-2">
                    <Link to={`/apps/${a.name}`} className="block font-mono text-xs">
                      <span className="text-[var(--color-ink)] group-hover:text-[var(--color-accent)]">
                        {a.name}
                      </span>
                      <span className="ml-2 text-[10px] text-[var(--color-faint)]">{a.namespace}</span>
                    </Link>
                  </td>
                  <td className="px-2 py-2">
                    <Pill level={levelOf(a.health)}>{a.health}</Pill>
                  </td>
                  <td className="px-2 py-2">
                    <Pill level={levelOf(a.syncStatus)}>{a.syncStatus}</Pill>
                  </td>
                  <td className="hidden px-2 py-2 md:table-cell">
                    <code className="font-mono text-[11px] text-[var(--color-muted)]">
                      {a.imageTag ? a.imageTag.slice(0, 7) : "—"}
                    </code>
                  </td>
                  <td className="hidden max-w-[22rem] truncate px-2 py-2 lg:table-cell">
                    <span className="font-mono text-[11px] text-[var(--color-muted)]" title={a.repo}>
                      {a.repo?.replace("https://github.com/", "") ?? "—"}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

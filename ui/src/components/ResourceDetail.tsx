import { useEffect, useState, useCallback } from "react";
import { X, RefreshCw } from "lucide-react";
import { Pill } from "@/components/Signal";
import { levelOf } from "@/lib/level";
import type { ResourceNode } from "@/types/api";

const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api";

type Tab = "logs" | "manifest";

/*
 * Detail for one node of the resource graph.
 *
 * Replaces the page-level log viewer, which always showed whichever pod
 * happened to be first and gave no way to reach any other. Selecting a node
 * here asks about that specific object.
 */
export function ResourceDetail({
  appName,
  node,
  onClose,
}: {
  appName: string;
  node: ResourceNode;
  onClose: () => void;
}) {
  const isPod = node.kind === "Pod";
  const [tab, setTab] = useState<Tab>(isPod ? "logs" : "manifest");
  const [logs, setLogs] = useState<string[]>([]);
  const [containers, setContainers] = useState<string[]>([]);
  const [container, setContainer] = useState<string>("");
  const [manifest, setManifest] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // A non-pod node has no logs, so never leave the tab on something empty.
  useEffect(() => {
    setTab(isPod ? "logs" : "manifest");
    setLogs([]);
    setManifest("");
    setError(null);
  }, [node.kind, node.name, isPod]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      if (tab === "logs" && isPod) {
        const q = container ? `?container=${encodeURIComponent(container)}` : "";
        const res = await fetch(
          `${API_BASE}/v1/apps/${appName}/pods/${encodeURIComponent(node.name)}/logs${q}`,
          { credentials: "include" },
        );
        if (!res.ok) throw new Error((await res.json().catch(() => null))?.message ?? `HTTP ${res.status}`);
        const body = await res.json();
        const d = body.data ?? body;
        setLogs(d.lines ?? []);
        setContainers(d.containers ?? []);
        if (!container && d.container) setContainer(d.container);
      } else {
        const res = await fetch(
          `${API_BASE}/v1/apps/${appName}/resources/${node.kind}/${encodeURIComponent(node.name)}`,
          { credentials: "include" },
        );
        if (!res.ok) throw new Error((await res.json().catch(() => null))?.message ?? `HTTP ${res.status}`);
        const body = await res.json();
        setManifest(JSON.stringify(body.data ?? body, null, 2));
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load");
    } finally {
      setLoading(false);
    }
  }, [appName, node.kind, node.name, tab, container, isPod]);

  useEffect(() => {
    load();
  }, [load]);

  // Escape closes, as it would in any panel that overlays content.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <aside className="rise flex h-[28rem] flex-col overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
      <header className="flex shrink-0 items-center gap-2 border-b border-[var(--color-line)] px-3 py-2">
        <span className="font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
          {node.kind}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs text-[var(--color-ink)]">
          {node.name}
        </span>
        {(node.health || node.status) && (
          <Pill level={levelOf(node.health || node.status)}>{node.health || node.status}</Pill>
        )}
        <button
          onClick={onClose}
          aria-label="Close detail"
          className="rounded-[var(--radius-sm)] p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-ink)]"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </header>

      <div className="flex shrink-0 items-center gap-1 border-b border-[var(--color-line)] px-2 py-1.5">
        {isPod && <TabButton active={tab === "logs"} onClick={() => setTab("logs")}>Logs</TabButton>}
        <TabButton active={tab === "manifest"} onClick={() => setTab("manifest")}>Manifest</TabButton>

        {tab === "logs" && containers.length > 1 && (
          <select
            value={container}
            onChange={(e) => setContainer(e.target.value)}
            className="ml-2 rounded-[var(--radius-sm)] border border-[var(--color-line)] bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-ink)]"
          >
            {containers.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
        )}

        <button
          onClick={load}
          aria-label="Reload"
          className="ml-auto rounded-[var(--radius-sm)] p-1 text-[var(--color-muted)] hover:text-[var(--color-ink)]"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-auto bg-[var(--color-bg)]">
        {error ? (
          <p className="p-3 font-mono text-xs text-[var(--color-crit)]">{error}</p>
        ) : tab === "logs" ? (
          logs.length === 0 ? (
            <p className="p-3 font-mono text-xs text-[var(--color-muted)]">
              {loading ? "Loading…" : "No output."}
            </p>
          ) : (
            <pre className="p-3 font-mono text-[11px] leading-relaxed text-[var(--color-ink-2)]">
              {logs.join("\n")}
            </pre>
          )
        ) : (
          <pre className="p-3 font-mono text-[11px] leading-relaxed text-[var(--color-ink-2)]">
            {manifest || (loading ? "Loading…" : "")}
          </pre>
        )}
      </div>
    </aside>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-[var(--radius-sm)] px-2 py-0.5 font-mono text-[11px] transition-colors ${
        active
          ? "bg-[var(--color-surface-2)] text-[var(--color-ink)]"
          : "text-[var(--color-muted)] hover:text-[var(--color-ink-2)]"
      }`}
    >
      {children}
    </button>
  );
}

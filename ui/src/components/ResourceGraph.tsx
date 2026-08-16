import { useMemo } from "react";
import { Box, Layers, Network, Server, FileText, Shield, Database } from "lucide-react";
import { levelOf, type Level } from "@/lib/level";
import type { ResourceNode } from "@/types/api";

/*
 * The application's resources as a left-to-right graph.
 *
 * Argo CD draws this shape because it answers the question you actually have —
 * what owns what, and which part of the chain is unhealthy — in one look. A
 * nested list gets you there too, but you have to read it. Depth is horizontal:
 * Application, then the resources it manages, then the pods those produce.
 */

const CARD_W = 218;
const CARD_H = 46;
const COL_GAP = 66;
const ROW_H = 62;

interface Placed {
  id: string;
  parent: string | null;
  node: ResourceNode;
  depth: number;
  y: number;
}

const KIND_ICON: Record<string, React.ComponentType<{ className?: string }>> = {
  Application: Layers,
  Deployment: Box,
  ReplicaSet: Layers,
  Pod: Server,
  Service: Network,
  ConfigMap: FileText,
  Secret: Shield,
  PersistentVolumeClaim: Database,
};

const TONE: Record<Level, { text: string; ring: string; dot: string }> = {
  ok: { text: "text-[var(--color-ok)]", ring: "border-[var(--color-ok)]/35", dot: "bg-[var(--color-ok)]" },
  warn: { text: "text-[var(--color-warn)]", ring: "border-[var(--color-warn)]/35", dot: "bg-[var(--color-warn)]" },
  crit: { text: "text-[var(--color-crit)]", ring: "border-[var(--color-crit)]/40", dot: "bg-[var(--color-crit)]" },
  info: { text: "text-[var(--color-info)]", ring: "border-[var(--color-info)]/35", dot: "bg-[var(--color-info)]" },
  idle: { text: "text-[var(--color-muted)]", ring: "border-[var(--color-line)]", dot: "bg-[var(--color-muted)]" },
};

export function ResourceGraph({
  appName,
  appHealth,
  nodes,
  selected,
  onSelect,
}: {
  appName: string;
  appHealth?: string;
  nodes: ResourceNode[];
  selected?: ResourceNode | null;
  onSelect?: (n: ResourceNode) => void;
}) {
  const { placed, width, height } = useMemo(() => layout(appName, appHealth, nodes), [appName, appHealth, nodes]);

  if (!nodes || nodes.length === 0) {
    return (
      <p className="px-3 py-8 text-center font-mono text-xs text-[var(--color-muted)]">
        No resources reported for this application.
      </p>
    );
  }

  const byId = new Map(placed.map((p) => [p.id, p]));

  return (
    // The graph is wider than the viewport on anything but a large screen, so it
    // scrolls in its own box rather than making the page scroll sideways.
    <div className="overflow-x-auto p-4">
      <div className="relative" style={{ width, height }}>
        <svg
          className="pointer-events-none absolute inset-0 overflow-visible"
          width={width}
          height={height}
          aria-hidden="true"
        >
          {placed.map((p) => {
            if (!p.parent) return null;
            const from = byId.get(p.parent);
            if (!from) return null;
            const x1 = from.depth * (CARD_W + COL_GAP) + CARD_W;
            const y1 = from.y + CARD_H / 2;
            const x2 = p.depth * (CARD_W + COL_GAP);
            const y2 = p.y + CARD_H / 2;
            const mid = x1 + (x2 - x1) / 2;
            return (
              <path
                key={`${p.parent}->${p.id}`}
                d={`M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`}
                fill="none"
                stroke="var(--color-line)"
                strokeWidth="1"
                strokeDasharray="3 3"
              />
            );
          })}
        </svg>

        {placed.map((p, i) => (
          <GraphCard
            key={p.id}
            placed={p}
            index={i}
            selected={
              !!selected && selected.kind === p.node.kind && selected.name === p.node.name
            }
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

function GraphCard({
  placed,
  index,
  selected,
  onSelect,
}: {
  placed: Placed;
  index: number;
  selected: boolean;
  onSelect?: (n: ResourceNode) => void;
}) {
  const { node, depth, y } = placed;
  // The synthetic Application root is a label, not a real object to inspect.
  const clickable = !!onSelect && node.kind !== "Application";
  const level = levelOf(node.health || node.status);
  const tone = TONE[level];
  const Icon = KIND_ICON[node.kind] ?? Box;

  return (
    <div
      role={clickable ? "button" : undefined}
      tabIndex={clickable ? 0 : undefined}
      onClick={clickable ? () => onSelect!(node) : undefined}
      onKeyDown={
        clickable
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect!(node);
              }
            }
          : undefined
      }
      className={`rise absolute flex items-center gap-2 rounded-[var(--radius-md)] border bg-[var(--color-surface)] px-2.5 transition-shadow ${tone.ring} ${
        clickable ? "cursor-pointer hover:border-[var(--color-accent)]" : ""
      } ${selected ? "ring-2 ring-[var(--color-accent)]" : ""}`}
      style={{
        left: depth * (CARD_W + COL_GAP),
        top: y,
        width: CARD_W,
        height: CARD_H,
        // Stagger the reveal outward from the root so the graph draws itself in
        // dependency order rather than all at once.
        animationDelay: `${Math.min(index * 35, 400)}ms`,
      }}
      title={node.detail ? `${node.kind}/${node.name} · ${node.detail}` : `${node.kind}/${node.name}`}
    >
      <Icon className={`h-4 w-4 shrink-0 ${tone.text}`} />

      <div className="flex min-w-0 flex-1 flex-col leading-tight">
        <span className="truncate font-mono text-[11px] text-[var(--color-ink)]">{node.name}</span>
        <span className="font-mono text-[9px] uppercase tracking-wide text-[var(--color-muted)]">
          {node.kind}
          {node.detail ? ` · ${node.detail}` : ""}
        </span>
      </div>

      <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${tone.dot}`} aria-label={node.health || node.status} />
    </div>
  );
}

/**
 * Assigns each node a column from its depth and a row from a post-order walk:
 * leaves take the next free row, parents centre on the span of their children.
 * That keeps edges short and stops branches overlapping.
 */
function layout(appName: string, appHealth: string | undefined, nodes: ResourceNode[]) {
  const placed: Placed[] = [];
  let row = 0;
  let maxDepth = 0;

  const walk = (node: ResourceNode, depth: number, parent: string | null, path: string): number => {
    const id = `${path}/${node.kind}:${node.name}`;
    maxDepth = Math.max(maxDepth, depth);
    const kids = node.children ?? [];

    let y: number;
    if (kids.length === 0) {
      y = row * ROW_H;
      row += 1;
    } else {
      const ys = kids.map((k) => walk(k, depth + 1, id, id));
      y = (ys[0] + ys[ys.length - 1]) / 2;
    }

    placed.push({ id, parent, node, depth, y });
    return y;
  };

  // A synthetic root, so the graph reads as "this application produces these"
  // rather than starting mid-chain.
  const rootId = "app";
  const ys = nodes.map((n) => walk(n, 1, rootId, rootId));
  const rootY = ys.length ? (Math.min(...ys) + Math.max(...ys)) / 2 : 0;

  placed.push({
    id: rootId,
    parent: null,
    node: { kind: "Application", name: appName, health: appHealth },
    depth: 0,
    y: rootY,
  });

  return {
    placed,
    width: (maxDepth + 1) * CARD_W + maxDepth * COL_GAP,
    height: Math.max(row, 1) * ROW_H,
  };
}

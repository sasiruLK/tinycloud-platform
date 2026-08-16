import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { Dot, Pill } from "@/components/Signal";
import { levelOf } from "@/lib/level";
import type { ResourceNode } from "@/types/api";

/*
 * The application's resources, nested the way they actually relate.
 *
 * Argo CD's Application records only a flat list, so a Deployment and the Pods
 * it ultimately owns appear as unrelated rows. The API rebuilds the hierarchy
 * from Kubernetes ownerReferences; this draws it, so "which pod belongs to what,
 * and is it healthy" is answerable by looking rather than by cross-referencing.
 */
export function ResourceTree({ nodes }: { nodes: ResourceNode[] }) {
  if (!nodes || nodes.length === 0) {
    return (
      <p className="px-3 py-6 text-center font-mono text-xs text-[var(--color-muted)]">
        No resources reported for this application.
      </p>
    );
  }
  return (
    <ul className="flex flex-col">
      {nodes.map((n) => (
        <Node key={`${n.kind}/${n.name}`} node={n} depth={0} />
      ))}
    </ul>
  );
}

function Node({ node, depth }: { node: ResourceNode; depth: number }) {
  const kids = node.children ?? [];
  const [open, setOpen] = useState(true);
  const level = levelOf(node.health || node.status);

  return (
    <li>
      <div
        className="flex items-center gap-2 border-b border-[var(--color-line-soft)] py-1.5 pr-3 transition-colors hover:bg-[var(--color-surface-2)]"
        style={{ paddingLeft: `${0.75 + depth * 1.25}rem` }}
      >
        {kids.length > 0 ? (
          <button
            onClick={() => setOpen((o) => !o)}
            aria-label={open ? "Collapse" : "Expand"}
            className="-ml-1 rounded-[var(--radius-sm)] p-0.5 text-[var(--color-muted)] hover:text-[var(--color-ink)]"
          >
            <ChevronRight
              className={`h-3 w-3 transition-transform ${open ? "rotate-90" : ""}`}
            />
          </button>
        ) : (
          <span className="w-[1.125rem]" />
        )}

        <Dot level={level} live={node.kind === "Pod" && node.health === "Healthy"} />

        <span className="w-[5.5rem] shrink-0 font-mono text-[10px] uppercase tracking-wide text-[var(--color-muted)]">
          {node.kind}
        </span>

        <span className="min-w-0 flex-1 truncate font-mono text-xs text-[var(--color-ink)]">
          {node.name}
        </span>

        {node.detail && (
          <span className="hidden font-mono text-[10px] text-[var(--color-faint)] sm:inline">
            {node.detail}
          </span>
        )}

        {(node.health || node.status) && (
          <Pill level={level}>{node.health || node.status}</Pill>
        )}
      </div>

      {open && kids.length > 0 && (
        <ul className="flex flex-col">
          {kids.map((k) => (
            <Node key={`${k.kind}/${k.name}`} node={k} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  );
}

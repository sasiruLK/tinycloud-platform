import { Badge } from "@/components/ui/badge";

interface StatusBadgeProps {
  status: string;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const lower = (status || "unknown").toLowerCase();

  // Health statuses
  if (lower === "healthy") {
    return <Badge variant="success">{status}</Badge>;
  }

  if (lower === "progressing") {
    return <Badge variant="warning">{status}</Badge>;
  }

  if (["degraded", "missing", "unknown", "suspended"].includes(lower)) {
    return <Badge variant="error">{status}</Badge>;
  }

  // Sync statuses
  if (lower === "synced") {
    return <Badge variant="success">{status}</Badge>;
  }

  if (lower === "outofsync") {
    return <Badge variant="warning">{status}</Badge>;
  }

  // Rollback statuses
  if (lower === "rollback") {
    return <Badge variant="error">{status}</Badge>;
  }

  if (lower === "normal") {
    return <Badge variant="secondary">{status}</Badge>;
  }

  // Build statuses
  if (lower === "queued") {
    return <Badge variant="secondary">{status}</Badge>;
  }

  if (lower === "succeeded") {
    return <Badge variant="success">{status}</Badge>;
  }

  if (lower === "failed") {
    return <Badge variant="error">{status}</Badge>;
  }

  // Resource statuses
  if (lower === "healthy" || lower === "running" || lower === "active") {
    return <Badge variant="success">{status}</Badge>;
  }

  return <Badge variant="secondary">{status}</Badge>;
}

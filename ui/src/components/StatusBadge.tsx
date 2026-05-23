import { Badge } from "@/components/ui/badge";

interface StatusBadgeProps {
  status: string;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const lower = status.toLowerCase();

  if (lower === "healthy" || lower === "synced") {
    return <Badge variant="success">{status}</Badge>;
  }

  if (lower === "progressing" || lower === "outofsync") {
    return <Badge variant="warning">{status}</Badge>;
  }

  if (lower === "degraded" || lower === "missing" || lower === "rollback") {
    return <Badge variant="error">{status}</Badge>;
  }

  return <Badge variant="secondary">{status}</Badge>;
}

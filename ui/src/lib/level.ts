/**
 * Status vocabulary.
 *
 * Argo CD, Kubernetes, OCI alarms and the build coordinator each use different
 * words for the same four states. This maps all of them onto one scale so the
 * UI can render status consistently regardless of which system reported it.
 *
 * Kept out of the component file so fast refresh keeps working — that rule
 * exists because mixing constants with components breaks hot reload state.
 */
export type Level = "ok" | "warn" | "crit" | "info" | "idle";

export function levelOf(raw: string | null | undefined): Level {
  const s = (raw ?? "").toLowerCase();
  if (["healthy", "synced", "ok", "succeeded", "running", "true", "active", "available"].includes(s)) return "ok";
  if (["progressing", "pending", "queued", "creating", "updating"].includes(s)) return "info";
  if (["degraded", "outofsync", "warning", "suspended", "missing"].includes(s)) return "warn";
  if (["failed", "error", "firing", "unhealthy", "crashloopbackoff", "false", "stopped"].includes(s)) return "crit";
  return "idle";
}

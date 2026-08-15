import { useState, useEffect, useCallback } from "react";
import type { InfraResponse } from "@/types/api";

const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api";

interface UseInfraResult {
  infra: InfraResponse | null;
  loading: boolean;
  /** Set when the endpoint is unreachable or errored. The page still renders
   *  whatever it last had, so a transient OCI hiccup does not blank the view. */
  error: string | null;
  refetch: () => void;
}

/**
 * Polls infrastructure health.
 *
 * 30s, matching the API's own 60s cache — polling faster only burns OCI
 * Monitoring quota for data that has not changed. Keeps the previous payload on
 * error rather than clearing it: a dashboard that goes blank the moment one
 * call fails is worse than one showing slightly old numbers with a warning.
 */
export function useInfra(): UseInfraResult {
  const [infra, setInfra] = useState<InfraResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchInfra = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/v1/infra`, { credentials: "include" });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        throw new Error(body?.message || `HTTP ${res.status}`);
      }
      const body = await res.json();
      setInfra(body.data ?? body);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load infrastructure");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchInfra();
    const t = setInterval(fetchInfra, 30_000);
    return () => clearInterval(t);
  }, [fetchInfra]);

  return { infra, loading, error, refetch: fetchInfra };
}

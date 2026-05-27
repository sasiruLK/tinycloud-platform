import { useState, useEffect, useCallback } from "react";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import type { App } from "@/types/api";

interface UseAppsResult {
  apps: App[];
  loading: boolean;
  error: string | null;
  errorRequestId: string | null;
  refetch: () => void;
}

export function useApps(): UseAppsResult {
  const [apps, setApps] = useState<App[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const fetchApps = useCallback(async () => {
    setLoading(true);
    setError(null);
    setErrorRequestId(null);
    try {
      const data = await apiClient.getApps(50);
      setApps(data);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load apps");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchApps();
    const interval = setInterval(fetchApps, 30000);
    return () => clearInterval(interval);
  }, [fetchApps]);

  return { apps, loading, error, errorRequestId, refetch: fetchApps };
}

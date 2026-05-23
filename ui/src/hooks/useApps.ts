import { useState, useEffect } from "react";
import { apiClient } from "@/api/client";
import type { App } from "@/types/api";

interface UseAppsResult {
  apps: App[];
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useApps(): UseAppsResult {
  const [apps, setApps] = useState<App[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchApps = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.getApps();
      setApps(data.apps);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load apps");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchApps();
  }, []);

  return { apps, loading, error, refetch: fetchApps };
}

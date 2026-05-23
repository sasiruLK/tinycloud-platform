import { useState, useEffect } from "react";
import { apiClient } from "@/api/client";
import type { AppDetail } from "@/types/api";

interface UseAppResult {
  app: AppDetail | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useApp(name: string): UseAppResult {
  const [app, setApp] = useState<AppDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchApp = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.getApp(name);
      setApp(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load app");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchApp();
  }, [name]);

  return { app, loading, error, refetch: fetchApp };
}

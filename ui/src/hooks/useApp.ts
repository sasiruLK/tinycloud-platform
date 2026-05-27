import { useState, useEffect, useCallback } from "react";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import type { AppDetail } from "@/types/api";

interface UseAppResult {
  app: AppDetail | null;
  loading: boolean;
  error: string | null;
  errorRequestId: string | null;
  refetch: () => void;
}

export function useApp(name: string): UseAppResult {
  const [app, setApp] = useState<AppDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const fetchApp = useCallback(async () => {
    if (!name) return;
    setLoading(true);
    setError(null);
    setErrorRequestId(null);
    try {
      const data = await apiClient.getApp(name);
      setApp(data);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load app");
      }
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    fetchApp();
    const interval = setInterval(fetchApp, 30000);
    return () => clearInterval(interval);
  }, [fetchApp]);

  return { app, loading, error, errorRequestId, refetch: fetchApp };
}

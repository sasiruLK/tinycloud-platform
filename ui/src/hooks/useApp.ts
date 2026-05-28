import { useState, useEffect, useCallback } from "react";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import type { AppDetail } from "@/types/api";

interface UseAppResult {
  app: AppDetail | null;
  loading: boolean;
  error: string | null;
  errorRequestId: string | null;
  pendingGitOps: boolean;
  refetch: () => void;
}

export function useApp(name: string, options?: { fastPoll?: boolean }): UseAppResult {
  const [app, setApp] = useState<AppDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);
  const [pendingGitOps, setPendingGitOps] = useState(false);

  const fetchApp = useCallback(async () => {
    if (!name) return;
    setLoading(true);
    setError(null);
    setErrorRequestId(null);
    try {
      const data = await apiClient.getApp(name);
      setApp(data);
      setPendingGitOps(false);
    } catch (err) {
      setApp(null);
      if (err instanceof ApiError && err.status === 404) {
        setPendingGitOps(true);
        setError(null);
        setErrorRequestId(null);
      } else if (err instanceof ApiError) {
        setPendingGitOps(false);
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setPendingGitOps(false);
        setError(err instanceof Error ? err.message : "Failed to load app");
      }
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    fetchApp();

    const intervalMs = options?.fastPoll || pendingGitOps ? 5000 : 30000;
    const interval = setInterval(fetchApp, intervalMs);
    return () => clearInterval(interval);
  }, [fetchApp, options?.fastPoll, pendingGitOps]);

  return { app, loading, error, errorRequestId, pendingGitOps, refetch: fetchApp };
}

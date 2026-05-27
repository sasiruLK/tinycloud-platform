import { useState, useCallback, useEffect } from "react";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import type { LogResponse } from "@/types/api";

interface UseLogsResult {
  logs: LogResponse | null;
  loading: boolean;
  error: string | null;
  errorRequestId: string | null;
  fetchLogs: (container?: string, tail?: number) => void;
}

export function useLogs(name: string): UseLogsResult {
  const [logs, setLogs] = useState<LogResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const fetchLogs = useCallback(async (container?: string, tail?: number) => {
    setLoading(true);
    setError(null);
    setErrorRequestId(null);
    try {
      const data = await apiClient.getLogs(name, container, tail);
      setLogs(data);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load logs");
      }
    } finally {
      setLoading(false);
    }
  }, [name]);

  useEffect(() => {
    fetchLogs(undefined, 100);
  }, [fetchLogs]);

  useEffect(() => {
    const interval = setInterval(() => {
      fetchLogs(undefined, 100);
    }, 30000);
    return () => clearInterval(interval);
  }, [fetchLogs]);

  return { logs, loading, error, errorRequestId, fetchLogs };
}

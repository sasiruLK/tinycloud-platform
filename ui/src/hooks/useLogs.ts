import { useState } from "react";
import { apiClient } from "@/api/client";
import type { LogResponse } from "@/types/api";

interface UseLogsResult {
  logs: LogResponse | null;
  loading: boolean;
  error: string | null;
  fetchLogs: (container?: string, tail?: number) => void;
}

export function useLogs(name: string): UseLogsResult {
  const [logs, setLogs] = useState<LogResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchLogs = async (container?: string, tail?: number) => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.getLogs(name, container, tail);
      setLogs(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load logs");
    } finally {
      setLoading(false);
    }
  };

  return { logs, loading, error, fetchLogs };
}

import { useState, useEffect, useCallback } from "react";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import type { RollbacksResponse } from "@/types/api";

interface UseRollbacksResult {
  rollbacks: RollbacksResponse | null;
  loading: boolean;
  error: string | null;
  errorRequestId: string | null;
  refetch: () => void;
}

export function useRollbacks(): UseRollbacksResult {
  const [rollbacks, setRollbacks] = useState<RollbacksResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const fetchRollbacks = useCallback(async () => {
    setLoading(true);
    setError(null);
    setErrorRequestId(null);
    try {
      const data = await apiClient.getRollbacks();
      setRollbacks(data);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load rollbacks");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRollbacks();
  }, [fetchRollbacks]);

  return { rollbacks, loading, error, errorRequestId, refetch: fetchRollbacks };
}

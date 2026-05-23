import { useState, useEffect } from "react";
import { apiClient } from "@/api/client";
import type { RollbacksResponse } from "@/types/api";

interface UseRollbacksResult {
  rollbacks: RollbacksResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

export function useRollbacks(): UseRollbacksResult {
  const [rollbacks, setRollbacks] = useState<RollbacksResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchRollbacks = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.getRollbacks();
      setRollbacks(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load rollbacks");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRollbacks();
  }, []);

  return { rollbacks, loading, error, refetch: fetchRollbacks };
}

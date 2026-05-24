import type {
  App,
  AppDetail,
  LogResponse,
  RollbackRequest,
  RollbackResponse,
  RestoreRequest,
  RestoreResponse,
  RollbacksResponse,
} from "@/types/api";

const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api";

async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const url = `${API_BASE}${path}`;
  const res = await fetch(url, {
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`HTTP ${res.status}: ${text}`);
  }
  return res.json();
}

export const apiClient = {
  getApps: () => api<{ apps: App[] }>("/v1/apps"),
  getApp: (name: string) => api<AppDetail>(`/v1/apps/${name}`),
  getLogs: (name: string, container?: string, tail?: number) =>
    api<LogResponse>(
      `/v1/apps/${name}/logs?${new URLSearchParams({
        container: container || "app",
        tail: String(tail || 100),
      })}`
    ),
  syncApp: (name: string) =>
    api<{ status: string }>(`/v1/apps/${name}/sync`, { method: "POST" }),
  rollbackApp: (name: string, body: RollbackRequest) =>
    api<RollbackResponse>(`/v1/apps/${name}/rollback`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  restoreApp: (name: string, body: RestoreRequest) =>
    api<RestoreResponse>(`/v1/apps/${name}/restore`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  getRollbacks: () => api<RollbacksResponse>("/v1/rollbacks"),
};

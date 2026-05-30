import { ApiError } from "./error";
import type {
  ApiSuccessResponse,
  ApiErrorResponse,
  AppsListData,
  AppDetail,
  LogResponse,
  RollbackRequest,
  RollbackResponse,
  RestoreRequest,
  RestoreResponse,
  RollbacksResponse,
  CreateAppRequest,
  CreateAppResponse,
  SuspendResponse,
  BuildJob,
  BuildLogsResponse,
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
    let errorBody: ApiErrorResponse;
    try {
      errorBody = await res.json();
    } catch {
      errorBody = {
        error: "unknown",
        message: `HTTP ${res.status}: ${res.statusText}`,
        requestId: res.headers.get("X-Request-Id") || "unknown",
        status: res.status,
      };
    }
    throw new ApiError(errorBody);
  }

  const body = (await res.json()) as ApiSuccessResponse<T>;
  return body.data;
}

export const apiClient = {
  getApps: (limit = 50, offset = 0) =>
    api<AppsListData>(`/v1/apps?limit=${limit}&offset=${offset}`).then(
      (res) => res.apps
    ),
  getApp: (name: string) => api<AppDetail>(`/v1/apps/${name}`),
  getLogs: (name: string, container?: string, tail?: number) => {
    const params = new URLSearchParams({
      tail: String(tail || 100),
    });
    if (container) {
      params.set("container", container);
    }
    return api<LogResponse>(`/v1/apps/${name}/logs?${params}`);
  },
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
  createApp: (body: CreateAppRequest) =>
    apiPost<CreateAppResponse>("/v1/apps", body),
  suspendApp: (name: string) =>
    api<SuspendResponse>(`/v1/apps/${name}/suspend`, { method: "POST" }),
  getBuild: (id: string) => api<BuildJob>(`/v1/builds/${id}`),
  getBuildLogs: (id: string, after = 0) =>
    api<BuildLogsResponse>(`/v1/builds/${id}/logs?after=${after}`),
};

async function apiPost<T>(path: string, body: unknown): Promise<T> {
  const url = `${API_BASE}${path}`;
  const res = await fetch(url, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let errorBody: ApiErrorResponse;
    try {
      errorBody = await res.json();
    } catch {
      errorBody = {
        error: "unknown",
        message: `HTTP ${res.status}: ${res.statusText}`,
        requestId: res.headers.get("X-Request-Id") || "unknown",
        status: res.status,
      };
    }
    throw new ApiError(errorBody);
  }

  const responseBody = (await res.json()) as ApiSuccessResponse<T>;
  return responseBody.data;
}

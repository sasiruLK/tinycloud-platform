// Phase 2 API response shapes
// All success responses: { data: T, requestId: string }
// All error responses: { error: string, message: string, requestId: string, status: number }

export interface ApiSuccessResponse<T> {
  data: T;
  requestId: string;
}

export interface ApiErrorResponse {
  error: string;
  message: string;
  requestId: string;
  status: number;
}

export interface PaginationMeta {
  limit: number;
  offset: number;
  total: number;
}

export interface AppsListData {
  apps: App[];
}

export interface App {
  name: string;
  namespace: string;
  health: string;
  syncStatus: string;
  revision: string;
  imageTag: string;
  targetRevision: string;
  lastSyncedAt: string;
  rollbackStatus: string;
  repo?: string;
  path?: string;
}

export interface AppDetail extends App {
  resources: Resource[];
}

export interface Resource {
  kind: string;
  name: string;
  status: string;
}

export interface LogResponse {
  pod: string;
  container: string;
  lines: string[];
}

export interface RollbackRequest {
  targetRevision: string;
  reason: string;
  initiatedBy: string;
}

export interface RollbackResponse {
  rollbackId: string;
  app: string;
  rollbackBranch: string;
  targetRevision: string;
  status: string;
}

export interface RestoreRequest {
  reason: string;
  initiatedBy: string;
}

export interface RestoreResponse {
  restoreId: string;
  app: string;
  restoredToRevision: string;
  status: string;
}

export interface RollbackEntry {
  ID: string;
  Type: string;
  Timestamp: string;
  TargetRevision: string;
  TargetImage: string;
  PreviousRevision: string;
  PreviousImage: string;
  RestoredToRevision: string;
  RestoredToImage: string;
  Reason: string;
  RollbackBranch: string;
  InitiatedBy: string;
}

export interface RollbacksResponse {
  apps: Record<string, {
    CurrentStatus: string;
    ActiveRollback: string | null;
    History: RollbackEntry[];
  }>;
  generatedAt: string;
  version: string;
}

export interface CreateAppRequest {
  name: string;
  repoUrl: string;
  ref: string;
  replicas: number;
  port: number;
  env?: Record<string, string>;
}

export interface CreateAppResponse {
  appName: string;
  buildId: string;
  status: "queued" | string;
}

export interface SuspendResponse {
  name: string;
  status: string;
  message: string;
}

export interface BuildJob {
  id: string;
  appName: string;
  repoUrl: string;
  ref: string;
  commitSha: string;
  framework: string;
  image: string;
  tag: string;
  status: "queued" | "running" | "succeeded" | "failed" | string;
  attempts: number;
  replicas: number;
  port: number;
  error?: string;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface BuildLogLine {
  sequence: number;
  timestamp: string;
  stream: string;
  message: string;
}

export interface BuildLogsResponse {
  lines: BuildLogLine[];
}

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

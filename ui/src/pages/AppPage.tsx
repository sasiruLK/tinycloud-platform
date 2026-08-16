import { useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useApp } from "@/hooks/useApp";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import { ResourceGraph } from "@/components/ResourceGraph";
import { ResourceDetail } from "@/components/ResourceDetail";
import type { ResourceNode } from "@/types/api";
import { StatusBadge } from "@/components/StatusBadge";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { getPlatformAppUrl } from "@/lib/tinycloud";
import { RefreshCw, GitBranch, FolderOpen, RotateCcw, Loader2, ExternalLink, PauseCircle, Hammer } from "lucide-react";

export function AppPage() {
  const [selectedNode, setSelectedNode] = useState<ResourceNode | null>(null);
  const { name } = useParams<{ name: string }>();
  const [searchParams] = useSearchParams();
  const expectPending = searchParams.get("pending") === "1";
  const { app, loading, error, errorRequestId, pendingGitOps, refetch } = useApp(name || "", {
    fastPoll: expectPending,
  });
  const [rollbackSha, setRollbackSha] = useState("");
  const [rollbackReason, setRollbackReason] = useState("");
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionErrorRequestId, setActionErrorRequestId] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);

  const handleSync = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionErrorRequestId(null);
    setActionSuccess(null);
    try {
      await apiClient.syncApp(name!);
      setActionSuccess("Sync triggered successfully");
      refetch();
    } catch (err) {
      if (err instanceof ApiError) {
        setActionError(err.getFriendlyMessage());
        setActionErrorRequestId(err.requestId);
      } else {
        setActionError(err instanceof Error ? err.message : "Sync failed");
      }
    } finally {
      setActionLoading(false);
    }
  };

  // Sync reconciles what git already says. Rebuild produces a new image from a
  // new commit — the only way a code change reaches the cluster.
  const handleRebuild = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionSuccess(null);
    try {
      const res = await apiClient.rebuildApp(name!);
      setActionSuccess(`Build ${res.buildId.slice(0, 8)} queued — watch it on the Builds page`);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Rebuild failed");
    } finally {
      setActionLoading(false);
    }
  };

  const handleRollback = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionErrorRequestId(null);
    setActionSuccess(null);
    try {
      await apiClient.rollbackApp(name!, {
        targetRevision: rollbackSha,
        reason: rollbackReason,
        initiatedBy: "ui-user",
      });
      setActionSuccess("Rollback triggered successfully");
      setRollbackSha("");
      setRollbackReason("");
      refetch();
    } catch (err) {
      if (err instanceof ApiError) {
        setActionError(err.getFriendlyMessage());
        setActionErrorRequestId(err.requestId);
      } else {
        setActionError(err instanceof Error ? err.message : "Rollback failed");
      }
    } finally {
      setActionLoading(false);
    }
  };

  const handleRestore = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionErrorRequestId(null);
    setActionSuccess(null);
    try {
      await apiClient.restoreApp(name!, {
        reason: "Restore from UI",
        initiatedBy: "ui-user",
      });
      setActionSuccess("Restore triggered successfully");
      refetch();
    } catch (err) {
      if (err instanceof ApiError) {
        setActionError(err.getFriendlyMessage());
        setActionErrorRequestId(err.requestId);
      } else {
        setActionError(err instanceof Error ? err.message : "Restore failed");
      }
    } finally {
      setActionLoading(false);
    }
  };

  const handleSuspend = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionErrorRequestId(null);
    setActionSuccess(null);
    try {
      await apiClient.suspendApp(name!);
      setActionSuccess("App suspended — scaling to 0 replicas via GitOps");
      refetch();
    } catch (err) {
      if (err instanceof ApiError) {
        setActionError(err.getFriendlyMessage());
        setActionErrorRequestId(err.requestId);
      } else {
        setActionError(err instanceof Error ? err.message : "Suspend failed");
      }
    } finally {
      setActionLoading(false);
    }
  };

  if (loading && !app && !pendingGitOps) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground animate-pulse">Loading app details...</div>
      </div>
    );
  }

  if (pendingGitOps && !app) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold tracking-tight">{name}</h1>
          <Button variant="outline" size="sm" onClick={refetch}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
        </div>
        <div className="rounded-lg border border-blue-200 bg-blue-50 p-6 dark:border-blue-900 dark:bg-blue-950/30">
          <div className="flex items-center gap-3">
            <Loader2 className="h-5 w-5 animate-spin text-blue-600" />
            <div>
              <p className="font-medium text-blue-900 dark:text-blue-100">
                Waiting for GitOps sync
              </p>
              <p className="text-sm text-blue-700 dark:text-blue-300 mt-1">
                Manifests committed to gitops-lab. ApplicationSet will create the Argo CD Application shortly.
                Polling every 5 seconds…
              </p>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold tracking-tight">{name}</h1>
          <Button variant="outline" size="sm" onClick={refetch}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Retry
          </Button>
        </div>
        <ErrorAlert message={error} requestId={errorRequestId} onRetry={refetch} />
      </div>
    );
  }

  if (!app) {
    return <div>App not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">{app.name}</h1>
          <p className="text-sm text-muted-foreground">{app.namespace}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={refetch}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
          <div className="flex gap-2">
            <StatusBadge status={app.health} />
            <StatusBadge status={app.syncStatus} />
          </div>
        </div>
      </div>

      {actionError && (
        <ErrorAlert message={actionError} requestId={actionErrorRequestId} />
      )}
      {actionSuccess && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/30">
          <p className="text-sm font-medium text-green-800 dark:text-green-200">
            {actionSuccess}
          </p>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <div className="text-sm text-muted-foreground">Revision</div>
              <code className="text-sm">{app.revision ? `${app.revision.slice(0, 12)}...` : "—"}</code>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Target</div>
              <div className="text-sm">{app.targetRevision}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Image</div>
              <code className="text-sm">{app.imageTag ? `${app.imageTag.slice(0, 12)}...` : "—"}</code>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Rollback</div>
              <div className="text-sm">{app.rollbackStatus}</div>
            </div>
          </div>

          {(app.repo || app.path) && (
            <div className="mt-4 pt-4 border-t grid grid-cols-1 md:grid-cols-2 gap-4">
              {app.repo && (
                <div>
                  <div className="text-sm text-muted-foreground flex items-center gap-1.5">
                    <GitBranch className="h-3.5 w-3.5" />
                    Repository
                  </div>
                  <a
                    href={app.repo}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-sm text-primary hover:underline break-all"
                  >
                    {app.repo}
                  </a>
                </div>
              )}
              {app.path && (
                <div>
                  <div className="text-sm text-muted-foreground flex items-center gap-1.5">
                    <FolderOpen className="h-3.5 w-3.5" />
                    Path
                  </div>
                  <div className="text-sm font-mono">{app.path}</div>
                </div>
              )}
            </div>
          )}

          <div className="mt-4 pt-4 border-t">
            <div className="text-sm text-muted-foreground flex items-center gap-1.5">
              <ExternalLink className="h-3.5 w-3.5" />
              Public URL
            </div>
            <a
              href={getPlatformAppUrl(app.name)}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-primary hover:underline break-all"
            >
              {getPlatformAppUrl(app.name)}
            </a>
          </div>

          <div className="mt-4 pt-4 border-t">
            <div className="text-sm text-muted-foreground">Destination</div>
            <div className="text-sm">
              {app.namespace} / {app.targetRevision}
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="flex gap-2">
        <Button onClick={handleSync} disabled={actionLoading}>
          <RotateCcw className="h-4 w-4 mr-1" />
          {actionLoading ? "Processing..." : "Sync"}
        </Button>

        <Button onClick={handleRebuild} disabled={actionLoading} variant="outline"
          title="Build the app again from the latest commit on its branch">
          <Hammer className="h-4 w-4 mr-1" />
          Rebuild
        </Button>

        <Dialog>
          <DialogTrigger asChild>
            <Button variant="outline">Rollback</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Rollback {app.name}</DialogTitle>
              <DialogDescription>
                Enter the target Git SHA to rollback to.
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-4">
              <Input
                placeholder="Target SHA (40 chars)"
                value={rollbackSha}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setRollbackSha(e.target.value)}
              />
              <Input
                placeholder="Reason"
                value={rollbackReason}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setRollbackReason(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button
                onClick={handleRollback}
                disabled={actionLoading || rollbackSha.length !== 40}
              >
                Confirm Rollback
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Button variant="secondary" onClick={handleRestore} disabled={actionLoading}>
          Restore to Main
        </Button>

        <Button variant="outline" onClick={handleSuspend} disabled={actionLoading}>
          <PauseCircle className="h-4 w-4 mr-1" />
          Suspend
        </Button>
      </div>

      <section className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--color-line)] bg-[var(--color-surface)]">
        <header className="flex items-center justify-between border-b border-[var(--color-line)] px-3 py-2">
          <h2 className="font-mono text-[11px] uppercase tracking-[0.12em] text-[var(--color-muted)]">
            Resources
          </h2>
          <span className="font-mono text-[10px] text-[var(--color-faint)]">
            ownership graph
          </span>
        </header>
        {/* Falls back to the flat list when the API predates the tree. */}
        <ResourceGraph
          selected={selectedNode}
          onSelect={setSelectedNode}
          appName={app.name}
          appHealth={app.health}
          nodes={
            app.tree?.length
              ? app.tree
              : (app.resources ?? []).map((r) => ({
                  kind: r.kind,
                  name: r.name,
                  status: r.status,
                }))
          }
        />
      </section>

      {selectedNode && (
        <ResourceDetail
          appName={app.name}
          node={selectedNode}
          onClose={() => setSelectedNode(null)}
        />
      )}
    </div>
  );
}

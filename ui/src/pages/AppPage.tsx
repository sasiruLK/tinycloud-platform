import { useState } from "react";
import { useParams } from "react-router-dom";
import { useApp } from "@/hooks/useApp";
import { apiClient } from "@/api/client";
import { LogViewer } from "@/components/LogViewer";
import { StatusBadge } from "@/components/StatusBadge";
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

export function AppPage() {
  const { name } = useParams<{ name: string }>();
  const { app, loading, error, refetch } = useApp(name || "");
  const [rollbackSha, setRollbackSha] = useState("");
  const [rollbackReason, setRollbackReason] = useState("");
  const [actionLoading, setActionLoading] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);

  const handleSync = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionSuccess(null);
    try {
      await apiClient.syncApp(name!);
      setActionSuccess("Sync triggered successfully");
      refetch();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Sync failed");
    } finally {
      setActionLoading(false);
    }
  };

  const handleRollback = async () => {
    setActionLoading(true);
    setActionError(null);
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
      setActionError(err instanceof Error ? err.message : "Rollback failed");
    } finally {
      setActionLoading(false);
    }
  };

  const handleRestore = async () => {
    setActionLoading(true);
    setActionError(null);
    setActionSuccess(null);
    try {
      await apiClient.restoreApp(name!, {
        reason: "Restore from UI",
        initiatedBy: "ui-user",
      });
      setActionSuccess("Restore triggered successfully");
      refetch();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "Restore failed");
    } finally {
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground">Loading app details...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="text-red-600">{error}</div>
        <Button onClick={refetch}>Retry</Button>
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
          <p className="text-muted-foreground">{app.namespace}</p>
        </div>
        <div className="flex gap-2">
          <StatusBadge status={app.health} />
          <StatusBadge status={app.syncStatus} />
        </div>
      </div>

      {actionError && (
        <div className="text-red-600 text-sm">{actionError}</div>
      )}
      {actionSuccess && (
        <div className="text-green-600 text-sm">{actionSuccess}</div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <div className="text-sm text-muted-foreground">Revision</div>
              <code className="text-sm">{app.revision.slice(0, 12)}...</code>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Target</div>
              <div className="text-sm">{app.targetRevision}</div>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Image</div>
              <code className="text-sm">{app.imageTag.slice(0, 12)}...</code>
            </div>
            <div>
              <div className="text-sm text-muted-foreground">Rollback Status</div>
              <div className="text-sm">{app.rollbackStatus}</div>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="flex gap-2">
        <Button onClick={handleSync} disabled={actionLoading}>
          {actionLoading ? "Processing..." : "Sync"}
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
      </div>

      <LogViewer appName={app.name} />

      <Card>
        <CardHeader>
          <CardTitle>Resources</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {app.resources?.map((resource) => (
              <div
                key={`${resource.kind}-${resource.name}`}
                className="flex items-center justify-between py-2 border-b last:border-0"
              >
                <span className="text-sm">
                  {resource.kind}/{resource.name}
                </span>
                <StatusBadge status={resource.status} />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

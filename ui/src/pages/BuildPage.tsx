import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import { ErrorAlert } from "@/components/ErrorAlert";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { BuildJob, BuildLogLine } from "@/types/api";
import { ExternalLink, RefreshCw } from "lucide-react";

const MAX_LOG_LINE_LENGTH = 4000;

function formatLogLine(line: BuildLogLine): string {
  const stream = line.stream ?? "stdout";
  const message = typeof line.message === "string" ? line.message : String(line.message ?? "");
  const trimmed = message.length > MAX_LOG_LINE_LENGTH
    ? `${message.slice(0, MAX_LOG_LINE_LENGTH)}… [truncated]`
    : message;
  return `[${stream}] ${trimmed}`;
}

function deployStatusLabel(status?: string): string {
  switch (status) {
    case "gitops_committed":
      return "GitOps committed";
    case "pending_argocd_application":
      return "Waiting for Argo CD";
    case "argocd_progressing":
      return "Argo CD progressing";
    case "argocd_out_of_sync":
      return "Argo CD out of sync";
    case "degraded":
      return "Degraded";
    case "deployed":
      return "Deployed";
    default:
      return status || "-";
  }
}

export function BuildPage() {
  const { id } = useParams<{ id: string }>();
  const [build, setBuild] = useState<BuildJob | null>(null);
  const [logs, setLogs] = useState<BuildLogLine[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);
  const [pollError, setPollError] = useState<string | null>(null);
  const hasLoadedRef = useRef(false);

  const fetchBuild = useCallback(async () => {
    if (!id) {
      setError("Missing build ID");
      setLoading(false);
      return;
    }

    const isInitialLoad = !hasLoadedRef.current;
    if (isInitialLoad) {
      setError(null);
      setErrorRequestId(null);
    }

    try {
      const [nextBuild, nextLogs] = await Promise.all([
        apiClient.getBuild(id),
        apiClient.getBuildLogs(id),
      ]);
      setBuild(nextBuild);
      setLogs(Array.isArray(nextLogs?.lines) ? nextLogs.lines : []);
      setPollError(null);
      hasLoadedRef.current = true;
    } catch (err) {
      const message = err instanceof ApiError
        ? err.getFriendlyMessage()
        : err instanceof Error
          ? err.message
          : "Failed to load build";

      if (isInitialLoad) {
        if (err instanceof ApiError) {
          setError(message);
          setErrorRequestId(err.requestId);
        } else {
          setError(message);
        }
      } else {
        setPollError(message);
      }
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    hasLoadedRef.current = false;
    setBuild(null);
    setLogs([]);
    setLoading(true);
    setError(null);
    setPollError(null);
    fetchBuild();
    const interval = setInterval(fetchBuild, 5000);
    return () => clearInterval(interval);
  }, [fetchBuild]);

  if (!id) {
    return <ErrorAlert message="Missing build ID in URL" />;
  }

  if (loading && !build) {
    return <div className="flex items-center justify-center h-64 text-muted-foreground animate-pulse">Loading build...</div>;
  }

  if (error) {
    return <ErrorAlert message={error} requestId={errorRequestId} onRetry={fetchBuild} />;
  }

  if (!build) {
    return <ErrorAlert message="Build not found" onRetry={fetchBuild} />;
  }

  const image = build.image && build.tag ? `${build.image}:${build.tag}` : "";
  const status = build.status || "unknown";
  const isDone = status === "succeeded" || status === "failed";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">{build.appName || "Build"}</h1>
          <p className="text-sm text-muted-foreground">{build.repoUrl || "-"} · {build.ref || "main"}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={fetchBuild}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
          <StatusBadge status={status} />
        </div>
      </div>

      {pollError && (
        <ErrorAlert message={`Refresh failed: ${pollError}`} onRetry={fetchBuild} />
      )}

      <Card>
        <CardHeader>
          <CardTitle>Build Details</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <div className="text-sm text-muted-foreground">Framework</div>
            <div className="text-sm">{build.framework || "-"}</div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Commit</div>
            <code className="text-sm">{build.commitSha ? `${build.commitSha.slice(0, 12)}...` : "-"}</code>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Attempts</div>
            <div className="text-sm">{build.attempts ?? 0}</div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Image</div>
            <code className="text-xs break-all">{image || "-"}</code>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Deployment Lifecycle</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <div className="text-sm text-muted-foreground">Deploy</div>
            <div className="text-sm">{deployStatusLabel(build.deployStatus)}</div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">GitOps Commit</div>
            <code className="text-sm">{build.gitopsCommitSha ? `${build.gitopsCommitSha.slice(0, 12)}...` : "-"}</code>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Argo CD</div>
            <div className="text-sm">
              {build.argoSyncStatus || "-"}{build.argoHealth ? ` / ${build.argoHealth}` : ""}
            </div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">GitOps Path</div>
            <code className="text-xs break-all">{build.gitopsPath || "-"}</code>
          </div>
        </CardContent>
      </Card>

      {build.error && (
        <ErrorAlert message={build.error} />
      )}

      {build.verificationError && (
        <ErrorAlert message={build.verificationError} />
      )}

      {status === "succeeded" && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/30">
          <p className="text-sm font-medium text-green-800 dark:text-green-200">
            Build succeeded. {deployStatusLabel(build.deployStatus)}
          </p>
          {build.appName && (
            <Button asChild size="sm" className="mt-3">
              <Link to={build.deployStatus === "deployed" ? `/apps/${build.appName}` : `/apps/${build.appName}?pending=1`}>
                <ExternalLink className="h-4 w-4 mr-1" />
                Open App
              </Link>
            </Button>
          )}
          {build.appUrl && (
            <Button asChild size="sm" variant="outline" className="mt-3 ml-2">
              <a href={build.appUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-1" />
                Open Route
              </a>
            </Button>
          )}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Build Logs{!isDone ? " · polling every 5s" : ""}</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="max-h-[480px] overflow-auto rounded-md bg-black p-4 text-xs text-white whitespace-pre-wrap break-words">
            {logs.length === 0 ? "Waiting for logs..." : logs.map(formatLogLine).join("\n")}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}

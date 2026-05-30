import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import { ErrorAlert } from "@/components/ErrorAlert";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { BuildJob, BuildLogLine } from "@/types/api";
import { ExternalLink, RefreshCw } from "lucide-react";

export function BuildPage() {
  const { id } = useParams<{ id: string }>();
  const [build, setBuild] = useState<BuildJob | null>(null);
  const [logs, setLogs] = useState<BuildLogLine[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const fetchBuild = async () => {
    if (!id) return;
    setError(null);
    setErrorRequestId(null);
    try {
      const [nextBuild, nextLogs] = await Promise.all([
        apiClient.getBuild(id),
        apiClient.getBuildLogs(id),
      ]);
      setBuild(nextBuild);
      setLogs(nextLogs.lines);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to load build");
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchBuild();
    const interval = setInterval(fetchBuild, 5000);
    return () => clearInterval(interval);
  }, [id]);

  if (loading && !build) {
    return <div className="flex items-center justify-center h-64 text-muted-foreground animate-pulse">Loading build...</div>;
  }

  if (error) {
    return <ErrorAlert message={error} requestId={errorRequestId} onRetry={fetchBuild} />;
  }

  if (!build) {
    return <div>Build not found</div>;
  }

  const image = build.image && build.tag ? `${build.image}:${build.tag}` : "";
  const isDone = build.status === "succeeded" || build.status === "failed";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">{build.appName}</h1>
          <p className="text-sm text-muted-foreground">{build.repoUrl} · {build.ref}</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={fetchBuild}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
          <StatusBadge status={build.status} />
        </div>
      </div>

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
            <div className="text-sm">{build.attempts}</div>
          </div>
          <div>
            <div className="text-sm text-muted-foreground">Image</div>
            <code className="text-xs break-all">{image || "-"}</code>
          </div>
        </CardContent>
      </Card>

      {build.error && (
        <ErrorAlert message={build.error} />
      )}

      {build.status === "succeeded" && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/30">
          <p className="text-sm font-medium text-green-800 dark:text-green-200">
            Build succeeded and manifests were committed to GitOps.
          </p>
          <Button asChild size="sm" className="mt-3">
            <Link to={`/apps/${build.appName}?pending=1`}>
              <ExternalLink className="h-4 w-4 mr-1" />
              Open App
            </Link>
          </Button>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Build Logs{!isDone ? " · polling every 5s" : ""}</CardTitle>
        </CardHeader>
        <CardContent>
          <pre className="max-h-[480px] overflow-auto rounded-md bg-black p-4 text-xs text-white">
            {logs.length === 0 ? "Waiting for logs..." : logs.map((line) => `[${line.stream}] ${line.message}`).join("\n")}
          </pre>
        </CardContent>
      </Card>
    </div>
  );
}

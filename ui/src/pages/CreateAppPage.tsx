import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Plus, Rocket } from "lucide-react";

const APP_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

export function CreateAppPage() {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [ref, setRef] = useState("main");
  const [replicas, setReplicas] = useState(1);
  const [port, setPort] = useState(8080);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);

  const nameValid = name.length > 0 && name.length <= 63 && APP_NAME_PATTERN.test(name);
  const previewUrl = nameValid
    ? `https://tinycloud-platform.duckdns.org/apps/${name}/`
    : null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setErrorRequestId(null);

    try {
      const build = await apiClient.createApp({
        name: name.trim(),
        repoUrl: repoUrl.trim(),
        ref: ref.trim() || "main",
        replicas,
        port,
      });
      navigate(`/builds/${build.buildId}`);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.getFriendlyMessage());
        setErrorRequestId(err.requestId);
      } else {
        setError(err instanceof Error ? err.message : "Failed to create app");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Create Application</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Build a GitHub repository, push it to GHCR, then deploy it via GitOps.
        </p>
      </div>

      {error && (
        <ErrorAlert message={error} requestId={errorRequestId} />
      )}

      <form onSubmit={handleSubmit}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Rocket className="h-5 w-5" />
              App Configuration
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label htmlFor="name" className="text-sm font-medium">App Name</label>
              <Input
                id="name"
                placeholder="my-app"
                value={name}
                onChange={(e) => setName(e.target.value.toLowerCase())}
                required
              />
              <p className="text-xs text-muted-foreground">
                DNS-1123 lowercase name (also used as namespace and service name)
              </p>
            </div>

            <div className="space-y-2">
              <label htmlFor="repoUrl" className="text-sm font-medium">GitHub Repository</label>
              <Input
                id="repoUrl"
                placeholder="https://github.com/user/my-app"
                value={repoUrl}
                onChange={(e) => setRepoUrl(e.target.value)}
                required
              />
              <p className="text-xs text-muted-foreground">Node.js and Go repositories are supported in Phase 1</p>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <label htmlFor="ref" className="text-sm font-medium">Ref</label>
                <Input
                  id="ref"
                  placeholder="main"
                  value={ref}
                  onChange={(e) => setRef(e.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <label htmlFor="replicas" className="text-sm font-medium">Replicas</label>
                <Input
                  id="replicas"
                  type="number"
                  min={1}
                  max={10}
                  value={replicas}
                  onChange={(e) => setReplicas(Number(e.target.value))}
                  required
                />
              </div>
              <div className="space-y-2">
                <label htmlFor="port" className="text-sm font-medium">Port</label>
                <Input
                  id="port"
                  type="number"
                  min={1}
                  max={65535}
                  value={port}
                  onChange={(e) => setPort(Number(e.target.value))}
                  required
                />
              </div>
            </div>

            {previewUrl && (
              <div className="rounded-md bg-muted p-3 text-sm">
                <span className="text-muted-foreground">Public URL: </span>
                <code className="text-xs">{previewUrl}</code>
              </div>
            )}

            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="outline" onClick={() => navigate("/apps")}>
                Cancel
              </Button>
              <Button type="submit" disabled={loading || !nameValid || !repoUrl.trim()}>
                <Plus className="h-4 w-4 mr-1" />
                {loading ? "Queueing..." : "Build App"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  );
}

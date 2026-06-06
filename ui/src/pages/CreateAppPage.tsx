import { useState, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { apiClient } from "@/api/client";
import { ApiError } from "@/api/error";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { ChevronDown, ChevronUp, Plus, Rocket, GitBranch } from "lucide-react";
import type { GitHubRepo } from "@/types/api";

const APP_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

export function CreateAppPage() {
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [repoUrl, setRepoUrl] = useState("");
  const [ref, setRef] = useState("main");
  const [replicas, setReplicas] = useState(1);
  const [port, setPort] = useState(8080);
  const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorRequestId, setErrorRequestId] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);

  // GitHub repo picker state
  const [repos, setRepos] = useState<GitHubRepo[]>([]);
  const [repoQuery, setRepoQuery] = useState("");
  const [repoLoading, setRepoLoading] = useState(false);
  const [showRepoDropdown, setShowRepoDropdown] = useState(false);
  const repoInputRef = useRef<HTMLInputElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const nameValid = name.length > 0 && name.length <= 63 && APP_NAME_PATTERN.test(name);
  const previewUrl = nameValid
    ? `https://tinycloud-platform.duckdns.org/apps/${name}/`
    : null;

  // Fetch GitHub repos on mount
  useEffect(() => {
    let cancelled = false;
    setRepoLoading(true);
    apiClient
      .getGitHubRepos()
      .then((res) => {
        if (!cancelled) setRepos(res.repos);
      })
      .catch(() => {
        // Silently fail — user can still type any URL
      })
      .finally(() => {
        if (!cancelled) setRepoLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node) &&
        repoInputRef.current &&
        !repoInputRef.current.contains(e.target as Node)
      ) {
        setShowRepoDropdown(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const filteredRepos = repoQuery
    ? repos.filter(
        (r) =>
          r.name.toLowerCase().includes(repoQuery.toLowerCase()) ||
          r.fullName.toLowerCase().includes(repoQuery.toLowerCase())
      )
    : repos;

  const handleSelectRepo = (repo: GitHubRepo) => {
    setRepoUrl(repo.url);
    setRepoQuery(repo.fullName);
    setRef(repo.defaultBranch || "main");
    setShowRepoDropdown(false);
  };

  const handleEnvAdd = () => {
    setEnvVars([...envVars, { key: "", value: "" }]);
  };

  const handleEnvRemove = (idx: number) => {
    setEnvVars(envVars.filter((_, i) => i !== idx));
  };

  const handleEnvChange = (idx: number, field: "key" | "value", val: string) => {
    const next = [...envVars];
    next[idx][field] = val;
    setEnvVars(next);
  };

  const buildEnvRecord = (): Record<string, string> | undefined => {
    const record: Record<string, string> = {};
    for (const { key, value } of envVars) {
      if (key.trim()) record[key.trim()] = value;
    }
    return Object.keys(record).length > 0 ? record : undefined;
  };

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
        env: buildEnvRecord(),
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
          Build a GitHub repository on the ARM builder, push it to OCIR, then deploy it via GitOps.
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
            {/* App Name */}
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

            {/* GitHub Repository */}
            <div className="space-y-2 relative">
              <label htmlFor="repoUrl" className="text-sm font-medium">GitHub Repository</label>
              <div className="relative">
                <Input
                  id="repoUrl"
                  ref={repoInputRef}
                  placeholder="Search or paste any GitHub URL"
                  value={repoQuery}
                  onChange={(e) => {
                    setRepoQuery(e.target.value);
                    setRepoUrl(e.target.value); // allow free-form paste
                    setShowRepoDropdown(true);
                  }}
                  onFocus={() => setShowRepoDropdown(true)}
                  required
                />
                {repoLoading && (
                  <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground animate-pulse">
                    Loading...
                  </span>
                )}
              </div>

              {showRepoDropdown && filteredRepos.length > 0 && (
                <div
                  ref={dropdownRef}
                  className="absolute z-10 w-full mt-1 max-h-60 overflow-auto rounded-md border bg-popover shadow-md"
                >
                  {filteredRepos.slice(0, 20).map((repo) => (
                    <button
                      key={repo.fullName}
                      type="button"
                      className="w-full text-left px-3 py-2 text-sm hover:bg-accent hover:text-accent-foreground border-b last:border-b-0"
                      onClick={() => handleSelectRepo(repo)}
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-medium">{repo.fullName}</span>
                        {repo.language && (
                          <span className="text-xs text-muted-foreground">{repo.language}</span>
                        )}
                      </div>
                      <div className="text-xs text-muted-foreground truncate">{repo.url}</div>
                    </button>
                  ))}
                </div>
              )}

              <p className="text-xs text-muted-foreground">
                Select from your repos or paste any public/private GitHub URL
              </p>
            </div>

            {/* Branch + Port (always visible) */}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <label htmlFor="ref" className="text-sm font-medium flex items-center gap-1">
                  <GitBranch className="h-3 w-3" />
                  Branch / Tag
                </label>
                <Input
                  id="ref"
                  placeholder="main"
                  value={ref}
                  onChange={(e) => setRef(e.target.value)}
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
                <p className="text-xs text-muted-foreground">
                  Your app must listen on <code>0.0.0.0:$PORT</code> (set via PORT env var). For Go/Node.js the platform auto-patches common patterns.
                </p>
              </div>
            </div>

            {/* Preview URL */}
            {previewUrl && (
              <div className="rounded-md bg-muted p-3 text-sm">
                <span className="text-muted-foreground">Public URL: </span>
                <code className="text-xs">{previewUrl}</code>
              </div>
            )}

            {/* Advanced Settings Toggle */}
            <div className="border rounded-lg">
              <button
                type="button"
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium hover:bg-muted/50 transition-colors"
              >
                <span>Advanced Settings</span>
                {showAdvanced ? (
                  <ChevronUp className="h-4 w-4 text-muted-foreground" />
                ) : (
                  <ChevronDown className="h-4 w-4 text-muted-foreground" />
                )}
              </button>

              {showAdvanced && (
                <div className="px-4 pb-4 space-y-4 border-t">
                  {/* Replicas */}
                  <div className="pt-3 space-y-2">
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

                  {/* Environment Variables */}
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Environment Variables</label>
                    {envVars.map((env, idx) => (
                      <div key={idx} className="flex items-center gap-2">
                        <Input
                          placeholder="KEY"
                          value={env.key}
                          onChange={(e) => handleEnvChange(idx, "key", e.target.value)}
                          className="flex-1"
                        />
                        <span className="text-muted-foreground">=</span>
                        <Input
                          placeholder="value"
                          value={env.value}
                          onChange={(e) => handleEnvChange(idx, "value", e.target.value)}
                          className="flex-1"
                        />
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => handleEnvRemove(idx)}
                        >
                          Remove
                        </Button>
                      </div>
                    ))}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={handleEnvAdd}
                    >
                      <Plus className="h-3 w-3 mr-1" />
                      Add Variable
                    </Button>
                  </div>
                </div>
              )}
            </div>

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

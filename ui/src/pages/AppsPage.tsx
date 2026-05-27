import { useApps } from "@/hooks/useApps";
import { AppCard } from "@/components/AppCard";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { RefreshCw } from "lucide-react";

export function AppsPage() {
  const { apps, loading, error, errorRequestId, refetch } = useApps();

  if (loading && apps.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground animate-pulse">Loading apps...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold tracking-tight">Applications</h1>
          <Button variant="outline" size="sm" onClick={refetch}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Refresh
          </Button>
        </div>
        <ErrorAlert message={error} requestId={errorRequestId} onRetry={refetch} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Applications</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {apps.length} app{apps.length !== 1 ? "s" : ""} · Auto-refreshes every 30s
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={refetch}>
          <RefreshCw className="h-4 w-4 mr-1" />
          Refresh
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {apps.map((app) => (
          <AppCard key={app.name} app={app} />
        ))}
      </div>
    </div>
  );
}

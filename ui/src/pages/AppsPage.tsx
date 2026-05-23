import { useApps } from "@/hooks/useApps";
import { AppCard } from "@/components/AppCard";
import { Button } from "@/components/ui/button";

export function AppsPage() {
  const { apps, loading, error, refetch } = useApps();

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground">Loading apps...</div>
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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold tracking-tight">Applications</h1>
        <Button variant="outline" onClick={refetch}>
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

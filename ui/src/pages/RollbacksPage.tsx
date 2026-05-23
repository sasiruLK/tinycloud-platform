import { useRollbacks } from "@/hooks/useRollbacks";
import { RollbackTable } from "@/components/RollbackTable";
import { Button } from "@/components/ui/button";

export function RollbacksPage() {
  const { rollbacks, loading, error, refetch } = useRollbacks();

  const allEntries = rollbacks
    ? Object.values(rollbacks.apps).flatMap((app) => app.History)
    : [];

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground">Loading rollbacks...</div>
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
        <h1 className="text-3xl font-bold tracking-tight">Rollback History</h1>
        <Button variant="outline" onClick={refetch}>
          Refresh
        </Button>
      </div>

      {allEntries.length === 0 ? (
        <div className="text-muted-foreground">No rollback history found.</div>
      ) : (
        <RollbackTable entries={allEntries} />
      )}
    </div>
  );
}

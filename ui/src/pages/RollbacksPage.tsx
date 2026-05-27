import { useRollbacks } from "@/hooks/useRollbacks";
import { RollbackTable } from "@/components/RollbackTable";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { RefreshCw } from "lucide-react";

export function RollbacksPage() {
  const { rollbacks, loading, error, errorRequestId, refetch } = useRollbacks();

  const allEntries = rollbacks
    ? Object.values(rollbacks.apps).flatMap((app) => app.History)
    : [];

  if (loading && !rollbacks) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-muted-foreground animate-pulse">Loading rollbacks...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold tracking-tight">Rollback History</h1>
          <Button variant="outline" size="sm" onClick={refetch}>
            <RefreshCw className="h-4 w-4 mr-1" />
            Retry
          </Button>
        </div>
        <ErrorAlert message={error} requestId={errorRequestId} onRetry={refetch} />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold tracking-tight">Rollback History</h1>
        <Button variant="outline" size="sm" onClick={refetch}>
          <RefreshCw className="h-4 w-4 mr-1" />
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

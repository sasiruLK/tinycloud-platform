import { useState } from "react";
import { useLogs } from "@/hooks/useLogs";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Terminal, RefreshCw } from "lucide-react";

interface LogViewerProps {
  appName: string;
}

export function LogViewer({ appName }: LogViewerProps) {
  const { logs, loading, error, errorRequestId, fetchLogs } = useLogs(appName);
  const [tail, setTail] = useState("100");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg flex items-center gap-2">
          <Terminal className="h-5 w-5" />
          Logs
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2 mb-4 flex-wrap">
          <Input
            placeholder="Tail"
            value={tail}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTail(e.target.value)}
            className="w-20"
            type="number"
            min={1}
            max={10000}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => fetchLogs(undefined, parseInt(tail) || 100)}
            disabled={loading}
          >
            <RefreshCw className={`h-4 w-4 mr-1 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
          {loading && !logs && (
            <span className="text-xs text-muted-foreground">Loading logs...</span>
          )}
        </div>

        {error && <ErrorAlert message={error} requestId={errorRequestId} />}

        {logs && (
          <div className="mt-4 bg-muted rounded p-4 overflow-auto max-h-96">
            <div className="text-xs text-muted-foreground mb-2">
              Pod: {logs.pod} | Container: {logs.container}
            </div>
            <pre className="text-xs whitespace-pre-wrap font-mono">
              {logs.lines.join("\n")}
            </pre>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

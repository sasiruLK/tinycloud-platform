import { useState } from "react";
import { useLogs } from "@/hooks/useLogs";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

interface LogViewerProps {
  appName: string;
}

export function LogViewer({ appName }: LogViewerProps) {
  const { logs, loading, error, fetchLogs } = useLogs(appName);
  const [container, setContainer] = useState("app");
  const [tail, setTail] = useState("100");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Logs</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex gap-2 mb-4">
          <Input
            placeholder="Container"
            value={container}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setContainer(e.target.value)}
            className="w-40"
          />
          <Input
            placeholder="Tail lines"
            value={tail}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setTail(e.target.value)}
            className="w-32"
            type="number"
          />
          <Button
            onClick={() => fetchLogs(container, parseInt(tail))}
            disabled={loading}
          >
            {loading ? "Loading..." : "Fetch Logs"}
          </Button>
        </div>

        {error && (
          <div className="text-red-600 text-sm mb-2">{error}</div>
        )}

        {logs && (
          <div className="bg-muted rounded p-4 overflow-auto max-h-96">
            <div className="text-xs text-muted-foreground mb-2">
              Container: {logs.container}
            </div>
            <pre className="text-xs whitespace-pre-wrap font-mono">
              {logs.logs}
            </pre>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

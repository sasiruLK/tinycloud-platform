import type { App } from "@/types/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "./StatusBadge";
import { Link } from "react-router-dom";

interface AppCardProps {
  app: App;
}

export function AppCard({ app }: AppCardProps) {
  const shortSha = app.imageTag.slice(0, 12);

  return (
    <Link to={`/apps/${app.name}`}>
      <Card className="hover:shadow-md transition-shadow cursor-pointer">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">{app.name}</CardTitle>
            <StatusBadge status={app.health} />
          </div>
          <p className="text-sm text-muted-foreground">{app.namespace}</p>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Sync Status</span>
              <StatusBadge status={app.syncStatus} />
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Image</span>
              <code className="text-xs bg-muted px-2 py-1 rounded">
                {shortSha}...
              </code>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Target</span>
              <span className="text-sm">{app.targetRevision}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

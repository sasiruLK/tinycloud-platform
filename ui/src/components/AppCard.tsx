import type { App } from "@/types/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "./StatusBadge";
import { Link } from "react-router-dom";
import { GitBranch, FolderOpen } from "lucide-react";

interface AppCardProps {
  app: App;
}

export function AppCard({ app }: AppCardProps) {
  const shortSha = app.imageTag.slice(0, 12);

  return (
    <Link to={`/apps/${app.name}`}>
      <Card className="hover:shadow-md transition-shadow cursor-pointer h-full">
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-lg">{app.name}</CardTitle>
            <StatusBadge status={app.health} />
          </div>
          <p className="text-sm text-muted-foreground">{app.namespace}</p>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Sync</span>
              <StatusBadge status={app.syncStatus} />
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Image</span>
              <code className="text-xs bg-muted px-2 py-1 rounded">
                {shortSha}...
              </code>
            </div>
            {app.repo && (
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <GitBranch className="h-3 w-3 flex-shrink-0" />
                <span className="truncate" title={app.repo}>
                  {app.repo.replace("https://github.com/", "")}
                </span>
              </div>
            )}
            {app.path && (
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <FolderOpen className="h-3 w-3 flex-shrink-0" />
                <span className="truncate" title={app.path}>
                  {app.path}
                </span>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

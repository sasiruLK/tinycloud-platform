import type { RollbackEntry } from "@/types/api";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";

interface RollbackTableProps {
  entries: RollbackEntry[];
}

export function RollbackTable({ entries }: RollbackTableProps) {
  const formatDate = (ts: string) => {
    const d = new Date(ts);
    return d.toLocaleString();
  };

  const shortSha = (sha: string) => (sha ? sha.slice(0, 12) + "..." : "-");

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Type</TableHead>
            <TableHead>App</TableHead>
            <TableHead>Timestamp</TableHead>
            <TableHead>Target SHA</TableHead>
            <TableHead>Reason</TableHead>
            <TableHead>By</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((entry) => (
            <TableRow key={entry.ID}>
              <TableCell>
                <Badge
                  variant={entry.Type === "rollback" ? "destructive" : "success"}
                >
                  {entry.Type}
                </Badge>
              </TableCell>
              <TableCell className="font-medium">
                {entry.ID.split("-")[1]}
              </TableCell>
              <TableCell className="text-sm text-muted-foreground">
                {formatDate(entry.Timestamp)}
              </TableCell>
              <TableCell>
                <code className="text-xs bg-muted px-1 py-0.5 rounded">
                  {shortSha(entry.TargetRevision || entry.RestoredToRevision)}
                </code>
              </TableCell>
              <TableCell className="max-w-xs truncate">
                {entry.Reason}
              </TableCell>
              <TableCell>{entry.InitiatedBy}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

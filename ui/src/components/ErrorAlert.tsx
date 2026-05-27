import { AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";

interface ErrorAlertProps {
  message: string;
  requestId?: string | null;
  onRetry?: () => void;
}

export function ErrorAlert({ message, requestId, onRetry }: ErrorAlertProps) {
  return (
    <div className="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900 dark:bg-red-950/30">
      <div className="flex items-start gap-3">
        <AlertCircle className="h-5 w-5 text-red-600 mt-0.5 flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium text-red-800 dark:text-red-200">
            {message}
          </p>
          {requestId && (
            <p className="mt-1 text-xs text-red-600/70 dark:text-red-400/70 font-mono">
              Request ID: {requestId}
            </p>
          )}
        </div>
        {onRetry && (
          <Button variant="outline" size="sm" onClick={onRetry} className="flex-shrink-0">
            Retry
          </Button>
        )}
      </div>
    </div>
  );
}

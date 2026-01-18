"use client";

import { useAppLogs } from "@/lib/hooks";
import { Skeleton } from "@/components/ui";
import { cn } from "@/lib/utils";

interface LogsTabProps {
  appId: string;
}

export function LogsTab({ appId }: LogsTabProps) {
  const { data, isLoading, error } = useAppLogs(appId);

  if (isLoading) {
    return (
      <div className="space-y-2 rounded-lg bg-muted/50 p-4">
        {Array.from({ length: 10 }).map((_, i) => (
          <Skeleton key={i} className="h-4 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/5 p-4 text-center">
        <p className="text-destructive">Failed to load logs</p>
      </div>
    );
  }

  if (!data?.logs || data.logs.length === 0) {
    return (
      <div className="rounded-lg border p-8 text-center">
        <p className="text-muted-foreground">No logs available</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Logs will appear here once your app starts generating output
        </p>
      </div>
    );
  }

  return (
    <div className="max-h-[600px] overflow-auto rounded-lg bg-muted/50 p-4 font-mono text-xs scrollbar-thin">
      {data.logs.map((log, index) => (
        <div
          key={index}
          className={cn(
            "py-0.5",
            log.level === "error" && "text-destructive",
            log.level === "warn" && "text-warning"
          )}
        >
          <span className="text-muted-foreground">
            [{new Date(log.timestamp).toLocaleString()}]
          </span>{" "}
          <span
            className={cn(
              "mr-2 rounded px-1 py-0.5 text-[10px] uppercase",
              log.level === "info" && "bg-blue-500/10 text-blue-500",
              log.level === "warn" && "bg-warning/10 text-warning",
              log.level === "error" && "bg-destructive/10 text-destructive",
              log.level === "debug" && "bg-muted text-muted-foreground"
            )}
          >
            {log.level}
          </span>
          {log.message}
        </div>
      ))}
    </div>
  );
}

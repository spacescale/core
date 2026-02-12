"use client";

import * as React from "react";
import { cn } from "../lib/utils";

/**
 * LogEntry — A single log line with timestamp, level badge, and message.
 *
 * Extracted from the SpaceScale log viewer page where each line displays
 * structured log data with color-coded severity levels.
 *
 * @example
 * ```tsx
 * <LogEntry
 *   timestamp="2024-01-15T10:30:00Z"
 *   level="info"
 *   message="Server started on port 8080"
 * />
 * ```
 */

export type LogLevel = "info" | "warn" | "error" | "debug" | "trace";

const levelColors: Record<LogLevel, string> = {
  info: "bg-blue-500/10 text-blue-500 border-blue-500/20",
  warn: "bg-amber-500/10 text-amber-500 border-amber-500/20",
  error: "bg-red-500/10 text-red-500 border-red-500/20",
  debug: "bg-purple-500/10 text-purple-500 border-purple-500/20",
  trace: "bg-gray-500/10 text-gray-400 border-gray-500/20",
};

export interface LogEntryProps extends React.HTMLAttributes<HTMLDivElement> {
  timestamp?: string;
  level: LogLevel;
  message: string;
  source?: string;
}

const LogEntry = React.forwardRef<HTMLDivElement, LogEntryProps>(
  ({ className, timestamp, level, message, source, ...props }, ref) => {
    const formattedTime = timestamp
      ? new Date(timestamp).toLocaleTimeString("en-US", {
          hour12: false,
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          fractionalSecondDigits: 3,
        })
      : undefined;

    return (
      <div
        ref={ref}
        className={cn(
          "group flex items-start gap-3 font-mono text-[13px] leading-relaxed py-0.5 px-2 hover:bg-muted/50 rounded transition-colors",
          className
        )}
        {...props}
      >
        {formattedTime && (
          <span className="shrink-0 select-none tabular-nums text-muted-foreground/60">
            {formattedTime}
          </span>
        )}
        <span
          className={cn(
            "inline-flex shrink-0 items-center rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
            levelColors[level]
          )}
        >
          {level}
        </span>
        {source && (
          <span className="shrink-0 text-muted-foreground/50">
            [{source}]
          </span>
        )}
        <span className="flex-1 break-all text-foreground/90">{message}</span>
      </div>
    );
  }
);
LogEntry.displayName = "LogEntry";

export { LogEntry };

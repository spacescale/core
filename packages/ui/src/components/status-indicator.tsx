"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../lib/utils";

/**
 * StatusIndicator — A pulsing dot that communicates live/deployment/error state.
 *
 * Seen across the SpaceScale dashboard, app list, and overview pages
 * wherever a resource's runtime status needs to be conveyed at a glance.
 *
 * @example
 * ```tsx
 * <StatusIndicator status="live" />
 * <StatusIndicator status="deploying" label="Deploying v2.3" />
 * <StatusIndicator status="error" size="lg" />
 * ```
 */

const statusIndicatorVariants = cva("relative inline-flex", {
  variants: {
    status: {
      live: "[--status-color:theme(colors.green.500)]",
      running: "[--status-color:theme(colors.green.500)]",
      deploying: "[--status-color:theme(colors.blue.500)]",
      building: "[--status-color:theme(colors.blue.500)]",
      queued: "[--status-color:theme(colors.yellow.500)]",
      warning: "[--status-color:theme(colors.amber.500)]",
      error: "[--status-color:theme(colors.red.500)]",
      failed: "[--status-color:theme(colors.red.500)]",
      stopped: "[--status-color:theme(colors.gray.500)]",
      idle: "[--status-color:theme(colors.gray.400)]",
    },
    size: {
      sm: "",
      default: "",
      lg: "",
    },
  },
  defaultVariants: {
    status: "live",
    size: "default",
  },
});

const dotSizes = {
  sm: "h-1.5 w-1.5",
  default: "h-2 w-2",
  lg: "h-3 w-3",
} as const;

const pingSizes = {
  sm: "h-1.5 w-1.5",
  default: "h-2 w-2",
  lg: "h-3 w-3",
} as const;

const animatedStatuses = new Set([
  "live",
  "running",
  "deploying",
  "building",
  "queued",
]);

export interface StatusIndicatorProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof statusIndicatorVariants> {
  label?: string;
}

const StatusIndicator = React.forwardRef<
  HTMLSpanElement,
  StatusIndicatorProps
>(({ className, status, size = "default", label, ...props }, ref) => {
  const sizeKey = size ?? "default";
  const shouldAnimate = animatedStatuses.has(status ?? "live");

  return (
    <span
      ref={ref}
      className={cn(
        statusIndicatorVariants({ status, size }),
        "inline-flex items-center gap-2",
        className
      )}
      {...props}
    >
      <span className="relative inline-flex">
        {shouldAnimate && (
          <span
            className={cn(
              "absolute inline-flex rounded-full bg-[var(--status-color)] opacity-75 animate-status-ping",
              pingSizes[sizeKey]
            )}
          />
        )}
        <span
          className={cn(
            "relative inline-flex rounded-full bg-[var(--status-color)]",
            dotSizes[sizeKey]
          )}
        />
      </span>
      {label && (
        <span className="text-sm text-muted-foreground">{label}</span>
      )}
    </span>
  );
});
StatusIndicator.displayName = "StatusIndicator";

export { StatusIndicator, statusIndicatorVariants };

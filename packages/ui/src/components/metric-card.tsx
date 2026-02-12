"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../lib/utils";

/**
 * MetricCard — A stats/KPI display card showing a value with label and optional trend.
 *
 * Seen on the SpaceScale dashboard homepage and application overview pages
 * for CPU, memory, requests, uptime, etc.
 *
 * @example
 * ```tsx
 * <MetricCard label="CPU Usage" value="42%" trend={{ value: 5, direction: "up" }} />
 * <MetricCard label="Memory" value="1.2 GB" variant="glass" icon={<Activity />} />
 * ```
 */

const metricCardVariants = cva(
  "rounded-xl border p-4 transition-all duration-300",
  {
    variants: {
      variant: {
        default: "bg-card text-card-foreground shadow-sm",
        glass: "glass",
        flat: "bg-muted/50 border-border/50",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export interface MetricCardProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof metricCardVariants> {
  label: string;
  value: string | number;
  description?: string;
  icon?: React.ReactNode;
  trend?: {
    value: number;
    direction: "up" | "down" | "flat";
  };
  footer?: React.ReactNode;
}

const MetricCard = React.forwardRef<HTMLDivElement, MetricCardProps>(
  (
    {
      className,
      variant,
      label,
      value,
      description,
      icon,
      trend,
      footer,
      ...props
    },
    ref
  ) => {
    const trendColor =
      trend?.direction === "up"
        ? "text-green-500"
        : trend?.direction === "down"
          ? "text-red-500"
          : "text-muted-foreground";

    const trendArrow =
      trend?.direction === "up"
        ? "\u2191"
        : trend?.direction === "down"
          ? "\u2193"
          : "\u2192";

    return (
      <div
        ref={ref}
        className={cn(metricCardVariants({ variant, className }))}
        {...props}
      >
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {label}
          </span>
          {icon && (
            <span className="text-muted-foreground">{icon}</span>
          )}
        </div>
        <div className="mt-2 flex items-baseline gap-2">
          <span className="text-2xl font-semibold tracking-tight">
            {value}
          </span>
          {trend && (
            <span className={cn("text-xs font-medium", trendColor)}>
              {trendArrow} {Math.abs(trend.value)}%
            </span>
          )}
        </div>
        {description && (
          <p className="mt-1 text-xs text-muted-foreground">{description}</p>
        )}
        {footer && (
          <div className="mt-3 border-t border-border/50 pt-3">{footer}</div>
        )}
      </div>
    );
  }
);
MetricCard.displayName = "MetricCard";

export { MetricCard, metricCardVariants };

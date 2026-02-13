"use client";

import * as React from "react";
import { cn } from "../lib/utils";

/**
 * EmptyState — A placeholder shown when a list or section has no content.
 *
 * Implied across many SpaceScale pages (projects, apps, logs) when data hasn't
 * been created yet.
 *
 * @example
 * ```tsx
 * <EmptyState
 *   icon={<Rocket className="h-12 w-12" />}
 *   title="No deployments yet"
 *   description="Deploy your first application to get started."
 *   action={<Button>Deploy Now</Button>}
 * />
 * ```
 */

export interface EmptyStateProps extends React.HTMLAttributes<HTMLDivElement> {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

const EmptyState = React.forwardRef<HTMLDivElement, EmptyStateProps>(
  ({ className, icon, title, description, action, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        "flex flex-col items-center justify-center rounded-lg border border-dashed px-6 py-12 text-center",
        className
      )}
      {...props}
    >
      {icon && (
        <div className="mb-4 text-muted-foreground">{icon}</div>
      )}
      <h3 className="text-lg font-semibold">{title}</h3>
      {description && (
        <p className="mt-1.5 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
);
EmptyState.displayName = "EmptyState";

export { EmptyState };

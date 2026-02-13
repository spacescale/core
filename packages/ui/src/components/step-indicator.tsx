"use client";

import * as React from "react";
import { Check } from "lucide-react";
import { cn } from "../lib/utils";

/**
 * StepIndicator — A multi-step progress tracker for wizards and deployment flows.
 *
 * Extracted from the SpaceScale deployment progress page showing sequential
 * build steps (clone, install, build, deploy) with their completion state.
 *
 * @example
 * ```tsx
 * <StepIndicator
 *   steps={[
 *     { label: "Clone", status: "completed" },
 *     { label: "Build", status: "active" },
 *     { label: "Deploy", status: "pending" },
 *   ]}
 * />
 * ```
 */

export interface Step {
  label: string;
  description?: string;
  status: "completed" | "active" | "pending" | "error";
  icon?: React.ReactNode;
}

export interface StepIndicatorProps
  extends React.HTMLAttributes<HTMLDivElement> {
  steps: Step[];
  orientation?: "horizontal" | "vertical";
}

const stepColors = {
  completed: "bg-green-500 border-green-500 text-white",
  active: "bg-primary border-primary text-primary-foreground animate-pulse-subtle",
  pending: "bg-muted border-border text-muted-foreground",
  error: "bg-red-500 border-red-500 text-white",
};

const lineColors = {
  completed: "bg-green-500",
  active: "bg-primary/50",
  pending: "bg-border",
  error: "bg-red-500/50",
};

const StepIndicator = React.forwardRef<HTMLDivElement, StepIndicatorProps>(
  ({ className, steps, orientation = "horizontal", ...props }, ref) => {
    const isVertical = orientation === "vertical";

    return (
      <div
        ref={ref}
        className={cn(
          "flex",
          isVertical ? "flex-col gap-0" : "items-center gap-0",
          className
        )}
        role="list"
        {...props}
      >
        {steps.map((step, index) => (
          <div
            key={index}
            className={cn(
              "flex",
              isVertical
                ? "flex-row items-start gap-3"
                : "flex-col items-center gap-2",
              !isVertical && index < steps.length - 1 && "flex-1"
            )}
            role="listitem"
          >
            <div
              className={cn(
                "flex items-center",
                !isVertical && "w-full"
              )}
            >
              <div
                className={cn(
                  "flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 text-xs font-semibold transition-all",
                  stepColors[step.status]
                )}
              >
                {step.status === "completed" ? (
                  <Check className="h-4 w-4" />
                ) : step.icon ? (
                  step.icon
                ) : (
                  index + 1
                )}
              </div>
              {!isVertical && index < steps.length - 1 && (
                <div
                  className={cn(
                    "mx-2 h-0.5 flex-1 rounded-full transition-colors",
                    lineColors[step.status]
                  )}
                />
              )}
            </div>
            <div
              className={cn(
                isVertical ? "pb-8" : "mt-1 text-center",
                isVertical &&
                  index < steps.length - 1 &&
                  "relative before:absolute before:left-[-19px] before:top-8 before:h-[calc(100%-1rem)] before:w-0.5 before:rounded-full",
                isVertical &&
                  index < steps.length - 1 &&
                  `before:${lineColors[step.status]}`
              )}
            >
              <p
                className={cn(
                  "text-sm font-medium",
                  step.status === "active" && "text-foreground",
                  step.status === "pending" && "text-muted-foreground",
                  step.status === "completed" && "text-foreground",
                  step.status === "error" && "text-red-500"
                )}
              >
                {step.label}
              </p>
              {step.description && (
                <p className="text-xs text-muted-foreground mt-0.5">
                  {step.description}
                </p>
              )}
            </div>
          </div>
        ))}
      </div>
    );
  }
);
StepIndicator.displayName = "StepIndicator";

export { StepIndicator };

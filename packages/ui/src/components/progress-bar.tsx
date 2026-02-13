"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "../lib/utils";

/**
 * ProgressBar — An animated progress indicator with optional label.
 *
 * Extracted from the SpaceScale deployment progress page where a linear bar
 * shows build/deploy completion with a gradient fill.
 *
 * @example
 * ```tsx
 * <ProgressBar value={65} />
 * <ProgressBar value={100} variant="success" label="Complete" />
 * <ProgressBar value={30} variant="glass" showValue />
 * ```
 */

const progressBarVariants = cva(
  "relative w-full overflow-hidden rounded-full bg-secondary",
  {
    variants: {
      size: {
        sm: "h-1",
        default: "h-2",
        lg: "h-3",
        xl: "h-4",
      },
      variant: {
        default: "[&>[data-bar]]:bg-primary",
        success: "[&>[data-bar]]:bg-green-500",
        warning: "[&>[data-bar]]:bg-amber-500",
        destructive: "[&>[data-bar]]:bg-red-500",
        gradient:
          "[&>[data-bar]]:bg-gradient-to-r [&>[data-bar]]:from-indigo-500 [&>[data-bar]]:to-purple-500",
        glass:
          "bg-white/5 [&>[data-bar]]:bg-gradient-to-r [&>[data-bar]]:from-indigo-500 [&>[data-bar]]:to-cyan-400",
      },
    },
    defaultVariants: {
      size: "default",
      variant: "default",
    },
  }
);

export interface ProgressBarProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof progressBarVariants> {
  value: number;
  max?: number;
  label?: string;
  showValue?: boolean;
  animated?: boolean;
}

const ProgressBar = React.forwardRef<HTMLDivElement, ProgressBarProps>(
  (
    {
      className,
      value,
      max = 100,
      label,
      showValue,
      animated = true,
      variant,
      size,
      ...props
    },
    ref
  ) => {
    const percentage = Math.min(Math.max((value / max) * 100, 0), 100);

    return (
      <div ref={ref} className={cn("w-full", className)} {...props}>
        {(label || showValue) && (
          <div className="mb-1.5 flex items-center justify-between text-xs">
            {label && (
              <span className="font-medium text-muted-foreground">
                {label}
              </span>
            )}
            {showValue && (
              <span className="font-medium tabular-nums">
                {Math.round(percentage)}%
              </span>
            )}
          </div>
        )}
        <div
          className={cn(progressBarVariants({ size, variant }))}
          role="progressbar"
          aria-valuenow={value}
          aria-valuemin={0}
          aria-valuemax={max}
        >
          <div
            data-bar=""
            className={cn(
              "h-full rounded-full transition-all",
              animated && "duration-500 ease-out"
            )}
            style={{ width: `${percentage}%` }}
          />
        </div>
      </div>
    );
  }
);
ProgressBar.displayName = "ProgressBar";

export { ProgressBar, progressBarVariants };

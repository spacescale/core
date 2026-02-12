"use client";

import * as React from "react";
import { LayoutGrid, List } from "lucide-react";
import { cn } from "../lib/utils";

/**
 * ViewToggle — A grid/list view switcher.
 *
 * Found on the SpaceScale projects and applications list pages allowing
 * users to toggle between card-grid and table-list layouts.
 *
 * @example
 * ```tsx
 * <ViewToggle value="grid" onValueChange={setView} />
 * ```
 */

export interface ViewToggleProps
  extends Omit<React.HTMLAttributes<HTMLDivElement>, "onChange"> {
  value: "grid" | "list";
  onValueChange: (value: "grid" | "list") => void;
}

const ViewToggle = React.forwardRef<HTMLDivElement, ViewToggleProps>(
  ({ className, value, onValueChange, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        "inline-flex items-center rounded-md border bg-muted p-0.5",
        className
      )}
      role="radiogroup"
      aria-label="View mode"
      {...props}
    >
      <button
        type="button"
        role="radio"
        aria-checked={value === "grid"}
        className={cn(
          "inline-flex items-center justify-center rounded-sm p-1.5 transition-colors",
          value === "grid"
            ? "bg-background text-foreground shadow-sm"
            : "text-muted-foreground hover:text-foreground"
        )}
        onClick={() => onValueChange("grid")}
      >
        <LayoutGrid className="h-4 w-4" />
        <span className="sr-only">Grid view</span>
      </button>
      <button
        type="button"
        role="radio"
        aria-checked={value === "list"}
        className={cn(
          "inline-flex items-center justify-center rounded-sm p-1.5 transition-colors",
          value === "list"
            ? "bg-background text-foreground shadow-sm"
            : "text-muted-foreground hover:text-foreground"
        )}
        onClick={() => onValueChange("list")}
      >
        <List className="h-4 w-4" />
        <span className="sr-only">List view</span>
      </button>
    </div>
  )
);
ViewToggle.displayName = "ViewToggle";

export { ViewToggle };

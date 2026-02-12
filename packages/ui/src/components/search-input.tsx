"use client";

import * as React from "react";
import { Search, X } from "lucide-react";
import { cn } from "../lib/utils";

/**
 * SearchInput — A text input with a built-in search icon and optional clear button.
 *
 * Found across the SpaceScale dashboard for filtering projects, repositories,
 * applications, and logs.
 *
 * @example
 * ```tsx
 * <SearchInput placeholder="Search projects..." onValueChange={setQuery} />
 * <SearchInput variant="glass" placeholder="Filter logs..." />
 * ```
 */

export interface SearchInputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  onValueChange?: (value: string) => void;
  onChange?: React.ChangeEventHandler<HTMLInputElement>;
  variant?: "default" | "glass";
  showClear?: boolean;
}

const SearchInput = React.forwardRef<HTMLInputElement, SearchInputProps>(
  (
    {
      className,
      onValueChange,
      onChange,
      variant = "default",
      showClear = true,
      value: controlledValue,
      defaultValue,
      ...props
    },
    ref
  ) => {
    const [internalValue, setInternalValue] = React.useState(
      (defaultValue as string) ?? ""
    );
    const isControlled = controlledValue !== undefined;
    const currentValue = isControlled ? String(controlledValue) : internalValue;

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
      if (!isControlled) setInternalValue(e.target.value);
      onChange?.(e);
      onValueChange?.(e.target.value);
    };

    const handleClear = () => {
      if (!isControlled) setInternalValue("");
      onValueChange?.("");
    };

    return (
      <div className="relative">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          ref={ref}
          type="text"
          value={currentValue}
          onChange={handleChange}
          className={cn(
            "flex h-10 w-full rounded-md border bg-background py-2 pl-9 pr-9 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
            variant === "default" && "border-input",
            variant === "glass" && "glass border-0",
            className
          )}
          {...props}
        />
        {showClear && currentValue && (
          <button
            type="button"
            onClick={handleClear}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
            aria-label="Clear search"
          >
            <X className="h-4 w-4" />
          </button>
        )}
      </div>
    );
  }
);
SearchInput.displayName = "SearchInput";

export { SearchInput };

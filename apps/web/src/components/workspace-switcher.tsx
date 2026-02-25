"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronDown, Pencil, Plus } from "lucide-react";
import { cn } from "@/lib/utils";

interface Workspace {
  id: string;
  name: string;
  active: boolean;
}

interface WorkspaceSwitcherProps {
  currentWorkspace: string;
  className?: string;
}

// Mock workspaces — replace with real API data once workspace entities exist
function useWorkspaces(currentWorkspace: string): Workspace[] {
  return [
    { id: "ws-1", name: currentWorkspace, active: true },
    { id: "ws-2", name: "workspace-02", active: false },
    { id: "ws-3", name: "workspace-03", active: false },
  ];
}

export function WorkspaceSwitcher({ currentWorkspace, className }: WorkspaceSwitcherProps) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const workspaces = useWorkspaces(currentWorkspace);

  // Close on click outside
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open]);

  return (
    <div ref={containerRef} className={cn("relative", className)}>
      {/* Trigger */}
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className={cn(
          "flex items-center gap-1 font-mono text-[12px] font-[400] tracking-wide",
          "text-emerald-600 dark:text-emerald-400",
          "bg-emerald-50 dark:bg-emerald-500/10",
          "border border-emerald-200/60 dark:border-emerald-500/20",
          "px-2 py-0.5 rounded",
          "transition-colors hover:bg-emerald-100/70 dark:hover:bg-emerald-500/15",
          "select-none cursor-pointer whitespace-nowrap"
        )}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span>user@{currentWorkspace}</span>
        <ChevronDown
          className={cn(
            "h-3 w-3 transition-transform duration-200",
            open && "rotate-180"
          )}
          strokeWidth={2.5}
        />
      </button>

      {/* Dropdown */}
      {open && (
        <div
          className={cn(
            "absolute top-full left-0 mt-1.5 z-50",
            "w-64 rounded-[8px] overflow-hidden",
            // Light: frost glass
            "bg-[rgba(255,255,255,0.98)] dark:bg-[rgba(24,27,38,0.98)]",
            "backdrop-blur-[40px]",
            "hairline-border border-black/[0.08] dark:border-white/[0.08]",
            "shadow-[0_10px_40px_-10px_rgba(0,0,0,0.15),_0_2px_10px_-2px_rgba(0,0,0,0.05)]",
            "dark:shadow-[0_10px_40px_-10px_rgba(0,0,0,0.5),_0_2px_10px_-2px_rgba(0,0,0,0.3)]",
            // Entry animation
            "animate-fade-in"
          )}
          role="listbox"
          aria-label="Switch workspace"
        >
          {/* Workspace list */}
          <div className="py-1.5">
            {workspaces.map((ws) => (
              <div
                key={ws.id}
                className={cn(
                  "w-full px-4 py-2 text-[11px] tracking-wide",
                  "flex items-center justify-between group/item",
                  "transition-colors",
                  ws.active
                    ? "text-foreground hover:bg-black/[0.03] dark:hover:bg-white/[0.03]"
                    : "text-muted-foreground hover:text-foreground hover:bg-black/[0.03] dark:hover:bg-white/[0.03]"
                )}
              >
                {/* Workspace selector */}
                <button
                  type="button"
                  role="option"
                  aria-selected={ws.active}
                  onClick={() => {
                    // Switch workspace — real navigation would go here
                    setOpen(false);
                  }}
                  className="flex items-center gap-2 text-left bg-transparent border-none p-0 cursor-pointer"
                >
                  <span className={ws.active ? "font-[500]" : "font-[200]"}>
                    {ws.name}
                  </span>
                  {ws.active && (
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 shadow-sm animate-pulse" />
                  )}
                </button>

                {/* Edit icon */}
                <button
                  type="button"
                  className={cn(
                    "opacity-0 group-hover/item:opacity-100 transition-opacity",
                    "text-muted-foreground hover:text-primary"
                  )}
                  onClick={() => {
                    // Edit workspace name
                  }}
                  aria-label={`Edit ${ws.name}`}
                >
                  <Pencil className="h-3 w-3" strokeWidth={1.5} />
                </button>
              </div>
            ))}
          </div>

          {/* Divider */}
          <div className="h-px bg-border/40 my-0.5 mx-0" />

          {/* Create workspace */}
          <div className="py-1.5">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                // Navigate to create workspace
              }}
              className={cn(
                "w-full text-left px-4 py-2 text-[11px] font-[200] tracking-wide",
                "flex items-center gap-2",
                "text-muted-foreground hover:text-primary hover:bg-black/[0.03] dark:hover:bg-white/[0.03]",
                "transition-colors"
              )}
            >
              <Plus className="h-3.5 w-3.5" strokeWidth={2} />
              <span>Create Workspace</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ChevronLeft,
  ChevronRight,
  Cpu,
  Database,
  LayoutGrid,
  LogOut,
  Settings,
  Zap,
} from "lucide-react";
import { useAuth } from "@/lib/hooks";
import { cn } from "@/lib/utils";

interface NavItem {
  href: string;
  label: string;
  icon: React.ElementType;
  disabled?: boolean;
}

interface SidebarProps {
  mobileOpen?: boolean;
  onClose?: () => void;
}

export function Sidebar({ mobileOpen = false, onClose }: SidebarProps) {
  const pathname = usePathname();
  const { logout, user } = useAuth();
  const [open, setOpen] = useState(true);

  const projectId = pathname.match(/^\/projects\/([^/]+)/)?.[1] ?? null;

  const resourceItems: NavItem[] = [
    { href: projectId ? `/projects/${projectId}` : "/projects", label: "Applications", icon: LayoutGrid },
    { href: projectId ? `/projects/${projectId}/workers`   : "#", label: "Workers",   icon: Cpu,      disabled: !projectId },
    { href: projectId ? `/projects/${projectId}/functions` : "#", label: "Functions", icon: Zap,      disabled: !projectId },
    { href: projectId ? `/projects/${projectId}/databases` : "#", label: "Databases", icon: Database, disabled: !projectId },
  ];

  const settingsHref = projectId ? `/projects/${projectId}/settings` : null;

  const workspaceName =
    user?.name?.toLowerCase().replace(/\s+/g, "-") ??
    user?.email?.split("@")[0]?.replace(/[^a-z0-9-]/g, "-") ??
    "my-workspace";

  const workspaceId = `${workspaceName.slice(0, 2)}-${Math.abs(
    workspaceName.split("").reduce((acc, c) => acc + c.charCodeAt(0), 0) % 9999
  ).toString().padStart(4, "0")}-x`;

  const sidebarContent = (
    <aside
      className={cn(
        "fixed left-0 top-[92px] z-40 h-[calc(100vh-92px)]",
        "border-r border-black/[0.06] dark:border-white/[0.05]",
        "bg-white/90 dark:bg-[rgba(11,13,20,0.95)]",
        "backdrop-blur-md",
        "flex flex-col justify-between",
        "transition-transform duration-300 ease-in-out",
        open ? "w-64" : "w-14",
        // Desktop: always visible. Mobile: slide in/out.
        "md:translate-x-0",
        mobileOpen ? "translate-x-0" : "-translate-x-full md:translate-x-0",
      )}
    >
      {/* Top: workspace + nav */}
      <div className="flex flex-col flex-1 py-6 overflow-y-auto sidebar-scroll">
        {open && (
          <div className="px-5 mb-8">
            <div className="flex items-center gap-3 text-foreground mb-1">
              <div
                className="w-2 h-2 rounded-full bg-primary flex-shrink-0"
                style={{ boxShadow: "0 0 10px var(--glow-primary)" }}
              />
              <span className="font-[500] text-sm tracking-wide truncate">
                {workspaceName}
              </span>
            </div>
            <div className="pl-5 text-[10px] text-muted-foreground font-mono">
              ID: {workspaceId}
            </div>
          </div>
        )}

        {open && (
          <div className="px-6 mb-3">
            <h3 className="text-[10px] uppercase tracking-widest text-muted-foreground/80 font-semibold">
              Resources
            </h3>
          </div>
        )}

        <nav className="space-y-0.5 px-3">
          {resourceItems.map((item) => {
            const isActive = item.disabled ? false
              : item.label === "Applications"
                ? pathname === "/projects" || (projectId ? pathname === `/projects/${projectId}` : false)
                : item.href !== "#" && pathname.startsWith(item.href);

            return (
              <Link
                key={item.label}
                href={item.disabled ? "#" : item.href}
                aria-disabled={item.disabled}
                tabIndex={item.disabled ? -1 : undefined}
                onClick={(e) => {
                  if (item.disabled) { e.preventDefault(); return; }
                  onClose?.();
                }}
                className={cn(
                  "flex items-center gap-3 px-3 py-2 rounded-md transition-all duration-150 group",
                  open ? "" : "justify-center",
                  isActive
                    ? [
                        "bg-white border border-gray-200/80 shadow-sm text-foreground",
                        "dark:bg-white/[0.04] dark:border-white/[0.05] dark:text-foreground dark:shadow-none",
                      ]
                    : item.disabled
                      ? "opacity-40 cursor-not-allowed text-muted-foreground"
                      : [
                          "text-muted-foreground hover:text-foreground",
                          "hover:bg-gray-100/50 dark:hover:bg-white/[0.02]",
                        ]
                )}
              >
                <item.icon
                  className={cn(
                    "flex-shrink-0 transition-colors",
                    open ? "h-[18px] w-[18px]" : "h-5 w-5",
                    isActive ? "text-primary" : "group-hover:text-primary transition-colors"
                  )}
                  strokeWidth={1.5}
                />
                {open && (
                  <span className={cn("text-sm truncate", isActive ? "font-[450]" : "font-[300]")}>
                    {item.label}
                  </span>
                )}
              </Link>
            );
          })}
        </nav>
      </div>

      {/* Bottom: Settings + Collapse + Sign out */}
      <div
        className={cn(
          "p-3 border-t border-black/[0.06] dark:border-white/[0.05]",
          "bg-white/30 dark:bg-[rgba(11,13,20,0.3)]"
        )}
      >
        <nav className="space-y-1">
          <Link
            href={settingsHref ?? "#"}
            aria-disabled={!settingsHref}
            onClick={(e) => {
              if (!settingsHref) { e.preventDefault(); return; }
              onClose?.();
            }}
            className={cn(
              "flex items-center gap-3 px-3 py-2 rounded-md transition-all group",
              open ? "" : "justify-center",
              settingsHref
                ? "text-muted-foreground hover:text-foreground hover:bg-gray-100/50 dark:hover:bg-white/[0.02]"
                : "opacity-40 cursor-not-allowed text-muted-foreground"
            )}
          >
            <Settings
              className="h-[18px] w-[18px] flex-shrink-0 group-hover:text-foreground transition-colors"
              strokeWidth={1.5}
            />
            {open && <span className="text-sm font-[300]">Settings</span>}
          </Link>

          {/* Collapse — desktop only */}
          <button
            type="button"
            onClick={() => setOpen(!open)}
            className={cn(
              "hidden md:flex w-full items-center px-3 py-2 rounded-md transition-all group",
              "text-muted-foreground hover:text-foreground hover:bg-gray-100/50 dark:hover:bg-white/[0.02]",
              open ? "justify-between" : "justify-center"
            )}
            aria-label={open ? "Collapse sidebar" : "Expand sidebar"}
          >
            {open && (
              <span className="text-[11px] font-[400] text-muted-foreground/70 group-hover:text-muted-foreground transition-colors">
                Collapse
              </span>
            )}
            {open ? (
              <ChevronLeft className="h-[18px] w-[18px] group-hover:-translate-x-0.5 transition-transform" strokeWidth={1.5} />
            ) : (
              <ChevronRight className="h-[18px] w-[18px] group-hover:translate-x-0.5 transition-transform" strokeWidth={1.5} />
            )}
          </button>

          <button
            type="button"
            onClick={logout}
            className={cn(
              "w-full flex items-center gap-3 px-3 py-2 rounded-md transition-all group",
              open ? "" : "justify-center",
              "text-muted-foreground/60 hover:text-destructive hover:bg-destructive/5"
            )}
          >
            <LogOut className="h-[18px] w-[18px] flex-shrink-0" strokeWidth={1.5} />
            {open && <span className="text-sm font-[300]">Sign out</span>}
          </button>
        </nav>
      </div>
    </aside>
  );

  return (
    <>
      {sidebarContent}
      {/* Mobile backdrop */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm md:hidden"
          aria-hidden="true"
          onClick={onClose}
        />
      )}
    </>
  );
}
